package valueobject

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	ErrSchemaInvalid     = errors.New("SCHEMA_INVALID")
	ErrUnsupportedOutput = errors.New("UNSUPPORTED_UNTIL_M3")
	ErrOutputKindInvalid = errors.New("OUTPUT_KIND_INVALID")
)

func ValidateStructuredPayload(kind OutputKind, payload []byte) error {
	if kind == OutputKindSceneDSL {
		return ErrUnsupportedOutput
	}
	value, err := decodeJSON(payload)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaInvalid, err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: payload must be an object", ErrSchemaInvalid)
	}
	switch kind {
	case OutputKindTopics:
		return validateTopics(object)
	case OutputKindScript:
		return validateScript(object)
	default:
		return fmt.Errorf("%w: %q", ErrOutputKindInvalid, kind)
	}
}

func CanonicalJSON(payload []byte) ([]byte, error) {
	value, err := decodeJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchemaInvalid, err)
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchemaInvalid, err)
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}

func decodeJSON(payload []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func validateTopics(object map[string]any) error {
	topics, ok := object["topics"].([]any)
	if !ok || len(topics) < 3 || len(topics) > 10 {
		return fmt.Errorf("%w: topics must contain 3 to 10 candidates", ErrSchemaInvalid)
	}
	for i, value := range topics {
		candidate, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: topics[%d] must be an object", ErrSchemaInvalid, i)
		}
		for _, key := range []string{"title", "angle", "hook"} {
			if _, ok := candidate[key].(string); !ok {
				return fmt.Errorf("%w: topics[%d].%s must be a string", ErrSchemaInvalid, i, key)
			}
		}
	}
	return nil
}

func validateScript(object map[string]any) error {
	if _, ok := object["title"].(string); !ok {
		return fmt.Errorf("%w: title must be a string", ErrSchemaInvalid)
	}
	scenes, ok := object["scenes"].([]any)
	if !ok || len(scenes) == 0 {
		return fmt.Errorf("%w: scenes must contain at least one scene", ErrSchemaInvalid)
	}
	for i, value := range scenes {
		scene, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: scenes[%d] must be an object", ErrSchemaInvalid, i)
		}
		if _, ok := scene["narration"].(string); !ok {
			return fmt.Errorf("%w: scenes[%d].narration must be a string", ErrSchemaInvalid, i)
		}
		if _, ok := scene["duration"].(json.Number); !ok {
			return fmt.Errorf("%w: scenes[%d].duration must be a number", ErrSchemaInvalid, i)
		}
	}
	return nil
}
