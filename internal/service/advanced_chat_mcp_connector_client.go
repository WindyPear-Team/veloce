package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type connectorMCPClient struct {
	userID uint
	runID  string
	device *AdvancedChatConnectorDevice
	server AdvancedChatMCPServer
}

func newConnectorMCPClient(userID uint, runID string, device *AdvancedChatConnectorDevice, server AdvancedChatMCPServer) *connectorMCPClient {
	return &connectorMCPClient{userID: userID, runID: runID, device: device, server: server}
}

func (client *connectorMCPClient) listTools(ctx context.Context) ([]mcpTool, error) {
	if client.device == nil || client.device.ID == "" {
		return nil, errors.New("connector MCP server requires a selected connector device")
	}
	result, err := client.callConnector(ctx, "mcp_list_tools", map[string]interface{}{
		"server": connectorMCPServerPayload(client.server),
	})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Tools []mcpTool `json:"tools"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return nil, fmt.Errorf("failed to decode connector MCP tools/list: %w", err)
	}
	return payload.Tools, nil
}

func (client *connectorMCPClient) callTool(ctx context.Context, name string, arguments map[string]interface{}) (mcpToolResult, error) {
	if client.device == nil || client.device.ID == "" {
		return mcpToolResult{}, errors.New("connector MCP server requires a selected connector device")
	}
	if arguments == nil {
		arguments = map[string]interface{}{}
	}
	result, err := client.callConnector(ctx, "mcp_call_tool", map[string]interface{}{
		"server":    connectorMCPServerPayload(client.server),
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return mcpToolResult{}, err
	}
	return parseMCPToolCallResult(json.RawMessage(result))
}

func (client *connectorMCPClient) callConnector(ctx context.Context, action string, payload map[string]interface{}) (string, error) {
	task, err := createAdvancedChatRawConnectorTask(client.userID, client.runID, advancedChatConnectorToolBinding{
		DeviceID: client.device.ID,
		Action:   action,
	}, payload, false)
	if err != nil {
		return "", err
	}
	return waitAdvancedChatConnectorTask(ctx, task.ID, client.userID)
}

func connectorMCPServerPayload(server AdvancedChatMCPServer) map[string]interface{} {
	return map[string]interface{}{
		"id":      server.ID,
		"name":    server.Name,
		"type":    advancedChatMCPTypeConnector,
		"command": server.Command,
		"args":    server.Args,
		"env":     server.Env,
		"cwd":     server.Cwd,
	}
}
