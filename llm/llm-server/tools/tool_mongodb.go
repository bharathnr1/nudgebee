package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"nudgebee/llm/config"
	"nudgebee/llm/relay"
	"nudgebee/llm/security"
	"nudgebee/llm/tools/core"
	"time"
)

// MVP read-only MongoDB troubleshooting tools. Each maps to a single forager
// MongoDB diagnostic command run through the proxy-agent path (the same path
// SSH/DB proxy datasources use — see executeSSHViaProxyAgent in
// common_relay.go). Unlike the SQL tools, MongoDB returns JSON documents, so
// the response is passed through as text/JSON rather than converted to a table.
const (
	ToolMongoServerStatus  = "mongodb_server_status_get"
	ToolMongoReplSetStatus = "mongodb_replset_status_get"
	ToolMongoCurrentOp     = "mongodb_current_op_get"
)

// mongoOp describes a single read-only MongoDB diagnostic operation: the tool
// name the AI calls, its description, and the Mongo command document forager
// should run.
type mongoOp struct {
	name        string
	description string
	// command is the MongoDB command document forager executes, e.g.
	// {"serverStatus": 1}. See buildMongoQueryParams for how it is wired into
	// the signed relay params.
	command map[string]any
}

var mongoOps = map[string]mongoOp{
	ToolMongoServerStatus: {
		name: ToolMongoServerStatus,
		description: `Returns MongoDB serverStatus — connections, memory, operation counters, network and asserts. ` +
			`Use this to inspect overall server health (e.g. connection saturation, memory pressure, op throughput). ` +
			`Read-only. Optional input: JSON object with an 'instance' field naming the MongoDB integration to target.`,
		command: map[string]any{"serverStatus": 1},
	},
	ToolMongoReplSetStatus: {
		name: ToolMongoReplSetStatus,
		description: `Returns MongoDB replSetGetStatus — replica-set membership, each member's state, and replication lag. ` +
			`Use this to check whether the replica set is healthy or a secondary is lagging. ` +
			`Read-only. Optional input: JSON object with an 'instance' field naming the MongoDB integration to target.`,
		command: map[string]any{"replSetGetStatus": 1},
	},
	ToolMongoCurrentOp: {
		name: ToolMongoCurrentOp,
		description: `Returns MongoDB currentOp — operations in progress right now. ` +
			`Use this to find long-running or slow operations. ` +
			`Read-only. Optional input: JSON object with an 'instance' field naming the MongoDB integration to target.`,
		command: map[string]any{"currentOp": 1},
	},
}

// mongoRelayExecute is the relay seam. It defaults to relay.Execute and is a
// package-level var so unit tests can substitute a deterministic fake without
// hitting the network. Tests must restore it afterwards.
var mongoRelayExecute = relay.Execute

func init() {
	for name := range mongoOps {
		n := name // capture
		core.RegisterNBToolFactory(n, func(accountId string) (core.NBTool, error) {
			return MongoDBTool{toolName: n}, nil
		})
	}
}

// MongoDBTool is a read-only MongoDB troubleshooting tool. One instance backs
// each of the three MVP operations, selected by toolName.
type MongoDBTool struct {
	toolName string
}

func (m MongoDBTool) op() mongoOp { return mongoOps[m.toolName] }

func (m MongoDBTool) Name() string { return m.toolName }

func (m MongoDBTool) GetType() core.NBToolType { return core.NBToolTypeTool }

func (m MongoDBTool) Description() string { return m.op().description }

func (m MongoDBTool) InputSchema() core.ToolSchema {
	return core.ToolSchema{
		Type: core.ToolSchemaTypeObject,
		Properties: map[string]core.ToolSchemaProperty{
			"instance": {
				Type:        core.ToolSchemaTypeString,
				Description: "Optional. Name/host of the MongoDB integration to target when several are configured.",
			},
		},
	}
}

func (m MongoDBTool) Call(nbRequestContext core.NbToolContext, input core.NBToolCallRequest) (core.NBToolResponse, error) {
	logger := nbRequestContext.Ctx.GetLogger()
	logger.Info("mongodb: executing read-only diagnostic", "tool", m.toolName)

	if nbRequestContext.ToolConfig.Name == "" && nbRequestContext.ToolConfig.Id == "" {
		return core.NBToolResponse{}, fmt.Errorf("no MongoDB integration configured for %s, please configure a MongoDB proxy integration", m.Name())
	}

	data, err := executeMongoViaProxyAgent(nbRequestContext, m.op().command, nbRequestContext.AccountId)
	if err != nil {
		// Propagate the forager error message so the LLM can act on it
		// (unreachable host, auth failure, etc.) — mirrors parseProxySSHResponse.
		logger.Error("mongodb: diagnostic failed", "tool", m.toolName, "error", err.Error())
		return core.NBToolResponse{
			Data:   err.Error(),
			Status: core.NBToolResponseStatusError,
		}, err
	}

	return core.NBToolResponse{
		Data:   data,
		Type:   core.NBToolResponseTypeText,
		Status: core.NBToolResponseStatusSuccess,
	}, nil
}

// buildMongoQueryParams builds the `params` object for the signed `mongo_query`
// relay action.
//
// !!! ASSUMPTION TO CONFIRM AGAINST FORAGER !!!
// The relay signer (relay-server/pkg/signing/signer.go) only fixes that the
// signed `mongo_query` payload is {action, datasource_id, params}. The *inner*
// shape of `params` is defined by the external forager
// (forager/pkg/proxy/mongodb/proxy.go), which is NOT in this repo and cannot be
// verified here. The mapping below is INFERRED from the SSH/DB proxy params
// shape (datasource_id + the command + timeout_ms). A reviewer with access to
// forager MUST confirm the exact key names ("command" / "timeout_ms") and that
// a command document like {"serverStatus": 1} is what forager expects.
// This builder is intentionally the single, isolated place to correct it.
func buildMongoQueryParams(datasourceKey string, command map[string]any, timeoutMs float64) map[string]any {
	return map[string]any{
		"datasource_id": datasourceKey,
		"command":       command,
		"timeout_ms":    timeoutMs,
	}
}

// executeMongoViaProxyAgent sends a signed `mongo_query` request to the forager
// agent via the relay, modeled on executeSSHViaProxyAgent. MongoDB datasources
// are always reached over the proxy-agent path.
func executeMongoViaProxyAgent(toolContext core.NbToolContext, command map[string]any, accountId string) (string, error) {
	// Fail fast if the tenant scope is missing: every proxy action is account-scoped,
	// and an empty accountId would execute without tenant isolation.
	if accountId == "" {
		return "", errors.New("accountId is required for tenant scoping")
	}

	datasourceKey := getConfigValue(toolContext.ToolConfig.Values, "datasource_key")
	if datasourceKey == "" {
		if toolContext.ToolConfig.Id != "" {
			datasourceKey = toolContext.ToolConfig.Id
		} else {
			return "", errors.New("MongoDB integration missing datasource_key config value")
		}
	}

	// MongoDB is a proxy-category integration with no k8s-native execution mode —
	// it is always reached via the forager (proxy) agent. Honor an explicit
	// agent_type override if the config sets one, otherwise default to "proxy".
	agentType := getConfigValue(toolContext.ToolConfig.Values, "agent_type")
	if agentType == "" {
		agentType = "proxy"
	}

	timeoutSeconds := config.Config.LlmServerRelayPodExecutionTimeoutSeconds
	params := buildMongoQueryParams(datasourceKey, command, float64(timeoutSeconds*1000))

	actionParam := relay.ActionExecuteBody{
		AccountID:    accountId,
		ActionName:   "mongo_query",
		ActionParams: params,
		AgentType:    agentType,
		Timeout:      time.Second * time.Duration(timeoutSeconds),
	}

	response, err := mongoRelayExecute(actionParam)
	if err != nil {
		return "", fmt.Errorf("proxy agent mongo_query failed: %w", err)
	}

	return parseProxyMongoResponse(response)
}

// parseProxyMongoResponse extracts the MongoDB JSON document(s) from the proxy
// agent's response. Unlike parseProxyDBResponse (SQL columns/rows → table),
// MongoDB returns JSON documents, so this is a JSON pass-through: it surfaces
// any forager error and otherwise returns the raw JSON string in response.data.
func parseProxyMongoResponse(response map[string]any) (string, error) {
	dataStr, ok := response["data"].(string)
	if !ok {
		return "", errors.New("proxy mongo_query response missing 'data' field")
	}

	// Inspect the payload only to surface a forager-side error; otherwise pass
	// the JSON through unchanged so document structure is preserved for the LLM.
	var mongoResult map[string]any
	if err := json.Unmarshal([]byte(dataStr), &mongoResult); err == nil && mongoResult != nil {
		if errMsg, ok := mongoResult["error"].(string); ok && errMsg != "" {
			return "", fmt.Errorf("proxy mongo_query error: %s", errMsg)
		}
	}

	return dataStr, nil
}

func (m MongoDBTool) IdentifyConfig(ctx core.NbToolContext, input core.NBToolCallRequest, availableConfigs []core.ToolConfig) (core.ToolConfig, error) {
	instanceName := ""
	if input.Arguments != nil {
		if inst, ok := input.Arguments["instance"].(string); ok {
			instanceName = inst
		}
	}
	if instanceName == "" {
		return core.ToolConfig{}, nil
	}
	for _, cfg := range availableConfigs {
		if cfg.Name == instanceName {
			return cfg, nil
		}
		if host := getConfigValue(cfg.Values, "host"); host == instanceName {
			return cfg, nil
		}
	}
	return core.ToolConfig{}, nil
}

func (m MongoDBTool) ConfigSchema(ctx *security.RequestContext) core.ToolConfigSchema {
	return core.ToolConfigSchema{
		Type:         core.ToolSchemaTypeObject,
		Required:     []string{"host"},
		ConfigType:   "mongodb_proxy",
		ConfigSource: core.ToolConfigSourceIntegration,
		Properties: map[string]core.ToolSchemaProperty{
			"host": {
				Type:        core.ToolSchemaTypeString,
				Description: "MongoDB host address",
			},
		},
	}
}
