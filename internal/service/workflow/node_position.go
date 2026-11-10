package workflow

import (
	"math"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

const generatedNodeGap = 36

func generatedNodePosition(nodes []entity.Node, origin entity.Position, nodeType valueobject.NodeType) entity.Position {
	size := entity.DefaultNodeSize(nodeType)
	stepX := size.Width + generatedNodeGap
	stepY := size.Height + generatedNodeGap
	for column := 0; column <= len(nodes); column++ {
		for row := 0; row <= len(nodes); row++ {
			position := entity.Position{X: origin.X + float64(column)*stepX, Y: origin.Y + float64(row)*stepY}
			if generatedNodePositionClear(nodes, position, size) {
				return position
			}
		}
	}

	rightmost := origin.X
	for _, node := range nodes {
		rightmost = math.Max(rightmost, node.Position.X+node.Size.Width)
	}
	return entity.Position{X: rightmost + generatedNodeGap, Y: origin.Y}
}

func generatedNodePositionClear(nodes []entity.Node, position entity.Position, size entity.NodeSize) bool {
	for _, node := range nodes {
		if position.X < node.Position.X+node.Size.Width+generatedNodeGap &&
			position.X+size.Width+generatedNodeGap > node.Position.X &&
			position.Y < node.Position.Y+node.Size.Height+generatedNodeGap &&
			position.Y+size.Height+generatedNodeGap > node.Position.Y {
			return false
		}
	}
	return true
}
