package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxArtifactChunkBytes = 16 * 1024

type RunChainOutput struct {
	RunID     string   `json:"runId"`
	TreeID    string   `json:"treeId"`
	ChainID   string   `json:"chainId"`
	ChainPath []string `json:"chainPath"`
	NodeID    string   `json:"nodeId"`
	Workspace string   `json:"workspace"`
}

type GetNodeInput struct {
	NodeID string `json:"nodeId" jsonschema:"Node ID on the current run path"`
}

type ReadArtifactInput struct {
	ArtifactID string `json:"artifactId" jsonschema:"Artifact ID referenced by a path ancestor"`
	Offset     int64  `json:"offset" jsonschema:"Zero-based UTF-8 byte offset"`
	Limit      int64  `json:"limit" jsonschema:"Maximum bytes to return, capped at 16384"`
}

type ReadArtifactOutput struct {
	ArtifactID string                 `json:"artifactId"`
	Kind       valueobject.OutputKind `json:"kind"`
	Offset     int64                  `json:"offset"`
	NextOffset int64                  `json:"nextOffset"`
	TotalBytes int64                  `json:"totalBytes"`
	Content    string                 `json:"content"`
	EOF        bool                   `json:"eof"`
}

type NodeTypeInfo struct {
	Type       valueobject.NodeType   `json:"type"`
	Parameters []string               `json:"parameters"`
	OutputKind valueobject.OutputKind `json:"outputKind"`
}

type ListNodeTypesOutput struct {
	Types []NodeTypeInfo `json:"types"`
}

type ValidateOutput struct {
	Valid   bool   `json:"valid"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type ScopedReplyOutput struct {
	Accepted       bool                      `json:"accepted"`
	PendingID      string                    `json:"pendingId,omitempty"`
	Status         valueobject.PendingStatus `json:"status,omitempty"`
	ResultNodeID   string                    `json:"resultNodeId,omitempty"`
	CreatedNodeIDs []string                  `json:"createdNodeIds,omitempty"`
	Error          string                    `json:"error,omitempty"`
}

type rawStructuredInput struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

type rawReplyInput struct {
	NodeID    string          `json:"node_id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	RequestID string          `json:"request_id,omitempty"`
}

func validateRunToolOptions(options Options) error {
	scope := options.RunScope
	if scope.RunID == "" || scope.ProjectID == "" || scope.TreeID == "" || scope.NodeID == "" || scope.ChainID != "c_"+scope.NodeID || len(scope.ChainPath) == 0 || scope.ChainPath[0] != scope.TreeID || scope.ChainPath[len(scope.ChainPath)-1] != scope.NodeID {
		return errors.New("invalid MCP run scope")
	}
	if scope.ContextHash == "" || scope.AncestorHash == "" || scope.ParamsHash == "" {
		return errors.New("MCP run scope is missing context hashes")
	}
	if scope.ExecutionSnapshot.Version != 0 {
		snapshot := scope.ExecutionSnapshot
		if snapshot.Validate() != nil || snapshot.ProjectID != scope.ProjectID || snapshot.TreeID != scope.TreeID || snapshot.NodeID != scope.NodeID || !slices.Equal(snapshot.ChainPath, scope.ChainPath) {
			return errors.New("invalid MCP execution snapshot")
		}
	}
	if options.RunTools == nil || options.RunLifecycle == nil || options.OnReply == nil {
		return errors.New("MCP run tools, lifecycle, and reply callback are required")
	}
	return nil
}

func (s *Server) withActiveRun(ctx context.Context, operation func() error) error {
	if s.options.RunScope == nil {
		return operation()
	}
	return s.options.RunLifecycle.WithActiveRun(ctx, *s.options.RunScope, operation)
}

func (s *Server) registerRunTools() {
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "get_chain", Description: "Return the current run identity, path, target node, and workspace."}, s.getRunChain)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "get_graph", Description: "Return only the nodes on the current run path, without sibling branches."}, s.getRunGraph)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "get_node", Description: "Read parameters and round metadata for a node on the current run path."}, s.getRunNode)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "list_node_types", Description: "List supported M2 node types, parameters, and output kinds."}, s.listNodeTypes)
	s.sdk.AddTool(&sdkmcp.Tool{
		Name:        "validate",
		Description: "Validate a topics or script payload without saving it.",
		InputSchema: rawObjectSchema(map[string]any{
			"kind":    map[string]any{"type": "string"},
			"payload": map[string]any{},
		}, "kind", "payload"),
	}, s.validateRun)
	sdkmcp.AddTool(s.sdk, &sdkmcp.Tool{Name: "read_artifact", Description: "Read an artifact referenced by a path ancestor in UTF-8 byte chunks up to 16 KiB."}, s.readRunArtifact)
	s.sdk.AddTool(&sdkmcp.Tool{
		Name:        "reply",
		Description: "Send a chat message or submit a topics/script result under the active run's frozen output contract. Valid output is committed by AudioUI; existing Script versions require confirmation.",
		InputSchema: rawObjectSchema(map[string]any{
			"node_id":    map[string]any{"type": "string"},
			"kind":       map[string]any{"type": "string"},
			"payload":    map[string]any{},
			"request_id": map[string]any{"type": "string"},
		}, "node_id", "kind", "payload"),
	}, s.replyRun)
}

func rawObjectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func (s *Server) getRunChain(ctx context.Context, _ *sdkmcp.CallToolRequest, _ GetChainInput) (*sdkmcp.CallToolResult, RunChainOutput, error) {
	var output RunChainOutput
	err := s.withActiveRun(ctx, func() error {
		scope := s.options.RunScope
		output = RunChainOutput{
			RunID: scope.RunID, TreeID: scope.TreeID, ChainID: scope.ChainID,
			ChainPath: append([]string(nil), scope.ChainPath...), NodeID: scope.NodeID,
			Workspace: s.options.Workspace,
		}
		return nil
	})
	return nil, output, err
}

func (s *Server) getRunGraph(ctx context.Context, _ *sdkmcp.CallToolRequest, _ GetChainInput) (*sdkmcp.CallToolResult, port.RunGraph, error) {
	var output port.RunGraph
	err := s.withActiveRun(ctx, func() error {
		var err error
		output, err = s.options.RunTools.ReadRunGraph(ctx, *s.options.RunScope)
		return err
	})
	return nil, output, err
}

func (s *Server) getRunNode(ctx context.Context, _ *sdkmcp.CallToolRequest, input GetNodeInput) (*sdkmcp.CallToolResult, port.RunNode, error) {
	var output port.RunNode
	err := s.withActiveRun(ctx, func() error {
		var err error
		output, err = s.options.RunTools.ReadRunNode(ctx, *s.options.RunScope, input.NodeID)
		return err
	})
	return nil, output, err
}

func (s *Server) listNodeTypes(ctx context.Context, _ *sdkmcp.CallToolRequest, _ GetChainInput) (*sdkmcp.CallToolResult, ListNodeTypesOutput, error) {
	output := ListNodeTypesOutput{Types: []NodeTypeInfo{
		{Type: valueobject.NodeTypeTopic, Parameters: []string{"idea", "direction", "candidateCount"}, OutputKind: valueobject.OutputKindTopics},
		{Type: valueobject.NodeTypeCandidate, Parameters: []string{"candidateSource"}, OutputKind: valueobject.OutputKindTopics},
		{Type: valueobject.NodeTypePrompt, Parameters: []string{"instruction", "outputKind", "constraints"}, OutputKind: valueobject.OutputKindScript},
		{Type: valueobject.NodeTypeAgent, Parameters: []string{}, OutputKind: valueobject.OutputKindScript},
		{Type: valueobject.NodeTypeScript, Parameters: []string{"scriptVersions"}, OutputKind: valueobject.OutputKindScript},
	}}
	err := s.withActiveRun(ctx, func() error { return nil })
	return nil, output, err
}

func (s *Server) validateRun(ctx context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
	var input rawStructuredInput
	if err := decodeRawArguments(request, &input); err != nil {
		return nil, err
	}
	if input.Kind == "" || len(input.Payload) == 0 {
		return rawToolError(errors.New("INVALID_INPUT: kind and payload are required")), nil
	}
	output := ValidateOutput{}
	err := s.withActiveRun(ctx, func() error {
		if len(input.Payload) > valueobject.MaxPendingPayloadSize {
			output.Code = "PENDING_PAYLOAD_TOO_LARGE"
			output.Message = "payload exceeds 256 KiB"
			return nil
		}
		if err := valueobject.ValidateStructuredPayload(valueobject.OutputKind(input.Kind), input.Payload); err != nil {
			output.Code = validationCode(err)
			output.Message = err.Error()
			return nil
		}
		output.Valid = true
		return nil
	})
	if err != nil {
		return rawToolError(err), nil
	}
	return rawToolResult(output), nil
}

func (s *Server) readRunArtifact(ctx context.Context, _ *sdkmcp.CallToolRequest, input ReadArtifactInput) (*sdkmcp.CallToolResult, ReadArtifactOutput, error) {
	var output ReadArtifactOutput
	err := s.withActiveRun(ctx, func() error {
		artifact, err := s.options.RunTools.ReadRunArtifact(ctx, *s.options.RunScope, input.ArtifactID)
		if err != nil {
			return err
		}
		output, err = artifactChunk(input.ArtifactID, artifact, input.Offset, input.Limit)
		return err
	})
	return nil, output, err
}

func artifactChunk(artifactID string, artifact port.RunArtifact, offset, limit int64) (ReadArtifactOutput, error) {
	data := artifact.Data
	if !utf8.Valid(data) {
		return ReadArtifactOutput{}, port.ErrArtifactCorrupt
	}
	if offset < 0 || offset > int64(len(data)) || (offset < int64(len(data)) && offset > 0 && !utf8.RuneStart(data[offset])) {
		return ReadArtifactOutput{}, errors.New("ARTIFACT_OFFSET_INVALID")
	}
	if limit <= 0 {
		return ReadArtifactOutput{}, errors.New("ARTIFACT_LIMIT_INVALID")
	}
	if limit > maxArtifactChunkBytes {
		limit = maxArtifactChunkBytes
	}
	end := offset + limit
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	for end > offset && end < int64(len(data)) && !utf8.RuneStart(data[end]) {
		end--
	}
	if end == offset && offset < int64(len(data)) {
		return ReadArtifactOutput{}, errors.New("ARTIFACT_LIMIT_INVALID: limit cannot contain the next UTF-8 character")
	}
	return ReadArtifactOutput{
		ArtifactID: artifactID, Kind: artifact.Kind, Offset: offset, NextOffset: end,
		TotalBytes: int64(len(data)), Content: string(data[offset:end]), EOF: end == int64(len(data)),
	}, nil
}

func (s *Server) replyRun(ctx context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
	var input rawReplyInput
	if err := decodeRawArguments(request, &input); err != nil {
		return nil, err
	}
	if input.NodeID == "" || input.Kind == "" || len(input.Payload) == 0 {
		return rawToolError(errors.New("INVALID_INPUT: node_id, kind, and payload are required")), nil
	}
	if len(input.Payload) > valueobject.MaxPendingPayloadSize {
		return rawToolError(errors.New("PENDING_PAYLOAD_TOO_LARGE")), nil
	}
	output := ScopedReplyOutput{}
	err := s.withActiveRun(ctx, func() error {
		scope := *s.options.RunScope
		if input.NodeID != scope.NodeID {
			if slices.Contains(scope.ChainPath, input.NodeID) {
				return port.ErrTurnNotActive
			}
			return port.ErrChainScopeDenied
		}
		switch input.Kind {
		case "message":
			var message struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(input.Payload, &message); err != nil || strings.TrimSpace(message.Text) == "" {
				return errors.New("INVALID_INPUT: message payload requires non-empty text")
			}
			s.options.OnReply(message.Text)
			output.Accepted = true
			return nil
		case string(valueobject.OutputKindTopics), string(valueobject.OutputKindScript):
		case string(valueobject.OutputKindSceneDSL):
			return errors.New("UNSUPPORTED_UNTIL_M3")
		default:
			return errors.New("OUTPUT_KIND_INVALID")
		}
		if scope.ExecutionSnapshot.Version != 0 && valueobject.OutputKind(input.Kind) != scope.ExecutionSnapshot.OutputContract.Kind {
			return errors.New("OUTPUT_KIND_MISMATCH")
		}
		pending, err := s.options.RunTools.SubmitRunReply(ctx, scope, valueobject.OutputKind(input.Kind), input.Payload, input.RequestID)
		if err != nil {
			return err
		}
		if pending.Status == valueobject.PendingPending || pending.Status == valueobject.PendingInvalid {
			if observer, ok := s.options.RunLifecycle.(port.RunPendingObserver); ok {
				if err := observer.RecordRunPending(ctx, scope, pending); err != nil {
					return err
				}
			}
		}
		output = ScopedReplyOutput{
			Accepted: true, PendingID: pending.ID, Status: pending.Status,
			ResultNodeID: pending.ResultNodeID,
			CreatedNodeIDs: append([]string(nil), pending.CreatedNodeIDs...), Error: pending.Error,
		}
		return nil
	})
	if err != nil {
		return rawToolError(err), nil
	}
	return rawToolResult(output), nil
}

func decodeRawArguments(request *sdkmcp.CallToolRequest, output any) error {
	if request == nil || request.Params == nil || len(request.Params.Arguments) == 0 {
		return errors.New("INVALID_INPUT: missing tool arguments")
	}
	if err := json.Unmarshal(request.Params.Arguments, output); err != nil {
		return fmt.Errorf("INVALID_INPUT: %w", err)
	}
	return nil
}

func validationCode(err error) string {
	switch {
	case errors.Is(err, valueobject.ErrUnsupportedOutput):
		return "UNSUPPORTED_UNTIL_M3"
	case errors.Is(err, valueobject.ErrOutputKindInvalid):
		return "OUTPUT_KIND_INVALID"
	case errors.Is(err, valueobject.ErrSchemaInvalid):
		return "SCHEMA_INVALID"
	default:
		return "INVALID_INPUT"
	}
}

func rawToolResult(output any) *sdkmcp.CallToolResult {
	data, err := json.Marshal(output)
	if err != nil {
		return rawToolError(err)
	}
	return &sdkmcp.CallToolResult{
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
		StructuredContent: output,
	}
}

func rawToolError(err error) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: err.Error()}},
		IsError: true,
	}
}
