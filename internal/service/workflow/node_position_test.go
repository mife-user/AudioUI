package workflow

import (
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

func TestGeneratedNodePositionAvoidsResizedNodesAndBatchSiblings(t *testing.T) {
	topicSize := entity.DefaultNodeSize(valueobject.NodeTypeTopic)
	largeNode := entity.Node{
		ID: "large", Type: valueobject.NodeTypeScript,
		Position: entity.Position{X: 536, Y: 0}, Size: entity.NodeSize{Width: 900, Height: 800},
	}
	nodes := []entity.Node{
		{ID: "topic", Type: valueobject.NodeTypeTopic, Position: entity.Position{X: 0, Y: 0}, Size: topicSize},
		largeNode,
	}
	candidateSize := entity.DefaultNodeSize(valueobject.NodeTypeCandidate)
	origin := entity.Position{X: topicSize.Width + generatedNodeGap, Y: 0}

	for index := 0; index < 4; index++ {
		requested := entity.Position{
			X: origin.X,
			Y: origin.Y + float64(index)*(candidateSize.Height+generatedNodeGap),
		}
		position := generatedNodePosition(nodes, requested, valueobject.NodeTypeCandidate)
		candidate := entity.Node{
			ID: "candidate", Type: valueobject.NodeTypeCandidate,
			Position: position, Size: candidateSize,
		}
		if !generatedNodePositionClear(nodes, position, candidateSize) {
			t.Fatalf("candidate %d overlaps an existing node: %+v", index, position)
		}
		nodes = append(nodes, candidate)
	}

	first := generatedNodePosition(nodes[:2], origin, valueobject.NodeTypeCandidate)
	second := generatedNodePosition(nodes[:2], origin, valueobject.NodeTypeCandidate)
	if first != second {
		t.Fatalf("position is not deterministic: first=%+v second=%+v", first, second)
	}
}

func TestGeneratedNodePositionUsesDefaultSizeForFreeSlot(t *testing.T) {
	topicSize := entity.DefaultNodeSize(valueobject.NodeTypeTopic)
	origin := entity.Position{X: topicSize.Width + generatedNodeGap, Y: 0}
	nodes := []entity.Node{{
		ID: "topic", Type: valueobject.NodeTypeTopic,
		Position: entity.Position{X: 0, Y: 0}, Size: topicSize,
	}}
	position := generatedNodePosition(nodes, origin, valueobject.NodeTypeCandidate)
	if position != origin {
		t.Fatalf("candidate position = %+v, want %+v", position, origin)
	}
}
