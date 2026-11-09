package workflow

import (
	"context"
	"fmt"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

func (s *Service) GetScriptVersion(ctx context.Context, nodeID string, version int, expectedProjectID string) (entity.ScriptVersionDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.ScriptVersionDetail{}, err
	}
	if nodeID == "" || expectedProjectID == "" || version < 1 {
		return entity.ScriptVersionDetail{}, fmt.Errorf("%w: project, node and positive version are required", ErrInvalidInput)
	}
	var detail entity.ScriptVersionDetail
	err := s.opts.Project.WithExclusive(ctx, func(access port.ProjectAccess) error {
		project, err := access.Current(ctx)
		if err != nil {
			return err
		}
		if project.ID != expectedProjectID {
			return fmt.Errorf("%w: expected %s, active %s", ErrProjectChanged, expectedProjectID, project.ID)
		}
		graph, err := entity.NewGraph(&project)
		if err != nil {
			return fmt.Errorf("图状态无效: %w", err)
		}
		if err := graph.Validate(); err != nil {
			return fmt.Errorf("图状态无效: %w", err)
		}
		node, err := requireNode(graph, nodeID, "node")
		if err != nil {
			return err
		}
		if node.Type != valueobject.NodeTypeScript {
			return fmt.Errorf("%w: node %s is not a script", ErrUnknownNodeType, nodeID)
		}
		var selected *entity.ScriptVersion
		for i := range node.ScriptVersions {
			if node.ScriptVersions[i].N == version {
				selected = &node.ScriptVersions[i]
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("%w: node %s version %d", ErrScriptVersionNotFound, nodeID, version)
		}
		if selected.ArtifactID == "" {
			return fmt.Errorf("%w: version has no artifact reference", ErrScriptVersionCorrupt)
		}
		if s.opts.Artifacts == nil {
			return ErrArtifactStoreUnavailable
		}
		store, err := s.opts.Artifacts.ForProject(ctx, project.ID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrArtifactStoreUnavailable, err)
		}
		artifact, payload, err := store.Get(ctx, selected.ArtifactID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrScriptVersionCorrupt, err)
		}
		if artifact.ID != selected.ArtifactID || len(payload) > valueobject.MaxPendingPayloadSize || !utf8.Valid(payload) {
			return fmt.Errorf("%w: artifact reference or payload is invalid", ErrScriptVersionCorrupt)
		}
		if err := valueobject.ValidateStructuredPayload(valueobject.OutputKindScript, payload); err != nil {
			return fmt.Errorf("%w: %v", ErrScriptVersionCorrupt, err)
		}
		detail = entity.ScriptVersionDetail{
			NodeID:  node.ID,
			Version: *selected,
			Payload: string(payload),
		}
		return nil
	})
	if err != nil {
		return entity.ScriptVersionDetail{}, err
	}
	return detail, nil
}
