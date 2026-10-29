package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

type Server struct {
	profile   Profile
	mu        sync.Mutex
	clientInfo map[string]any
}

func NewServer(profile Profile) *Server {
	return &Server{
		profile:    profile,
		clientInfo: make(map[string]any),
	}
}

type jsonrpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      any              `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Result  any            `json:"result,omitempty"`
	Error   *jsonrpcError  `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      map[string]any `json:"clientInfo"`
}

func (s *Server) Serve(ctx context.Context) error {
	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := readRequest(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read request: %w", err)
		}

		resp := s.handleRequest(ctx, req)
		if resp != nil {
			if err := writeResponse(writer, resp); err != nil {
				return fmt.Errorf("write response: %w", err)
			}
		}
	}
}

func readRequest(r *bufio.Reader) (*jsonrpcRequest, error) {
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	var req jsonrpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	return &req, nil
}

func writeResponse(w io.Writer, resp *jsonrpcResponse) error {
	body, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleRequest(ctx context.Context, req *jsonrpcRequest) *jsonrpcResponse {
	if req.JSONRPC != "2.0" {
		return errorResponse(req.ID, -32600, "Invalid JSON-RPC version")
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		return nil
	case "ping":
		return s.handlePing(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return errorResponse(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req *jsonrpcRequest) *jsonrpcResponse {
	var params initializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	s.mu.Lock()
	s.clientInfo = params.ClientInfo
	s.mu.Unlock()

	recordClientInfo(params.ClientInfo)

	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "filex",
				"version": "0.1.0",
			},
		},
	}
}

func (s *Server) handlePing(req *jsonrpcRequest) *jsonrpcResponse {
	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{},
	}
}

func (s *Server) handleToolsList(req *jsonrpcRequest) *jsonrpcResponse {
	tools := getToolDefinitions(s.profile)
	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"tools": tools,
		},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req *jsonrpcRequest) *jsonrpcResponse {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	if !s.profile.AllowsTool(params.Name) {
		return &jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"isError": true,
				"content": []map[string]any{
					{
						"type": "text",
						"text": fmt.Sprintf("tool %q not allowed in %s profile", params.Name, s.profile),
					},
				},
			},
		}
	}

	result, err := dispatchTool(ctx, s.profile, params.Name, params.Arguments)
	if err != nil {
		return &jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"isError": true,
				"content": []map[string]any{
					{
						"type": "text",
						"text": err.Error(),
					},
				},
			},
		}
	}

	resultJSON, _ := json.Marshal(result)
	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"isError": false,
			"content": []map[string]any{
				{
					"type": "text",
					"text": string(resultJSON),
				},
			},
		},
	}
}

func errorResponse(id any, code int, message string) *jsonrpcResponse {
	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonrpcError{
			Code:    code,
			Message: message,
		},
	}
}

func recordClientInfo(info map[string]any) {
	if info == nil {
		return
	}
	name, _ := info["name"].(string)
	version, _ := info["version"].(string)
	if name != "" {
		_ = journalRecord("mcp_initialize", "", fmt.Sprintf("client=%s version=%s", name, version))
	}
}

func journalRecord(op, path, detail string) error {
	return nil
}
