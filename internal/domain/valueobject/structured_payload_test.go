package valueobject

import (
	"bytes"
	"errors"
	"testing"
)

func TestValidateStructuredPayload(t *testing.T) {
	validTopics := []byte(`{"topics":[{"title":"a","angle":"a","hook":"a","extra":1},{"title":"b","angle":"b","hook":"b"},{"title":"c","angle":"c","hook":"c"}],"extra":true}`)
	if err := ValidateStructuredPayload(OutputKindTopics, validTopics); err != nil {
		t.Fatalf("valid topics: %v", err)
	}
	validScript := []byte(`{"title":"脚本","scenes":[{"narration":"旁白","duration":0,"extra":true}]}`)
	if err := ValidateStructuredPayload(OutputKindScript, validScript); err != nil {
		t.Fatalf("valid script: %v", err)
	}
	for _, test := range []struct {
		kind    OutputKind
		payload []byte
	}{
		{OutputKindTopics, []byte(`{"topics":[{"title":"a","angle":"a","hook":"a"}]}`)},
		{OutputKindTopics, []byte(`{"topics":[{"title":"a","angle":"a","hook":"a"},{"title":"b","angle":"b","hook":"b"},{"title":3,"angle":"c","hook":"c"}]}`)},
		{OutputKindScript, []byte(`{"title":"脚本","scenes":[{"narration":"旁白","duration":"1"}]}`)},
		{OutputKindScript, []byte(`{"title":"脚本","scenes":[]}`)},
		{OutputKindScript, []byte(`{"title":"脚本","scenes":[{"narration":"旁白","duration":1}]} {}`)},
	} {
		if err := ValidateStructuredPayload(test.kind, test.payload); !errors.Is(err, ErrSchemaInvalid) {
			t.Errorf("ValidateStructuredPayload(%s) error = %v, want SCHEMA_INVALID", test.kind, err)
		}
	}
	if err := ValidateStructuredPayload(OutputKindSceneDSL, []byte(`{}`)); !errors.Is(err, ErrUnsupportedOutput) {
		t.Fatalf("scene_dsl error = %v, want UNSUPPORTED_UNTIL_M3", err)
	}
}

func TestCanonicalJSONSortsKeysWithoutLosingNumberPrecision(t *testing.T) {
	first, err := CanonicalJSON([]byte(` {"z":1,"a":90071992547409931234567890,"html":"<"} `))
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalJSON([]byte(`{"html":"<","a":90071992547409931234567890,"z":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || string(first) != `{"a":90071992547409931234567890,"html":"<","z":1}` {
		t.Fatalf("canonical values differ or lost precision: %s / %s", first, second)
	}
}
