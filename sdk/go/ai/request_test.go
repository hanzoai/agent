package ai

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithSystem(t *testing.T) {
	req := &Request{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	err := WithSystem("You are a helpful assistant")(req)
	assert.NoError(t, err)
	assert.Len(t, req.Messages, 2)
	assert.Equal(t, "system", req.Messages[0].Role)
	assert.Equal(t, "You are a helpful assistant", req.Messages[0].Content)
	assert.Equal(t, "user", req.Messages[1].Role)
}

func TestWithModel(t *testing.T) {
	req := &Request{}

	err := WithModel("gpt-3.5-turbo")(req)
	assert.NoError(t, err)
	assert.Equal(t, "gpt-3.5-turbo", req.Model)
}

func TestWithTemperature(t *testing.T) {
	req := &Request{}

	temp := 0.9
	err := WithTemperature(temp)(req)
	assert.NoError(t, err)
	assert.NotNil(t, req.Temperature)
	assert.Equal(t, temp, *req.Temperature)
}

func TestWithMaxTokens(t *testing.T) {
	req := &Request{}

	tokens := 2000
	err := WithMaxTokens(tokens)(req)
	assert.NoError(t, err)
	assert.NotNil(t, req.MaxTokens)
	assert.Equal(t, tokens, *req.MaxTokens)
}

func TestWithStream(t *testing.T) {
	req := &Request{}

	err := WithStream()(req)
	assert.NoError(t, err)
	assert.True(t, req.Stream)
}

func TestWithJSONMode(t *testing.T) {
	req := &Request{}

	err := WithJSONMode()(req)
	assert.NoError(t, err)
	assert.NotNil(t, req.ResponseFormat)
	assert.Equal(t, "json_object", req.ResponseFormat.Type)
	assert.Nil(t, req.ResponseFormat.JSONSchema)
}

func TestWithSchema_WithStruct(t *testing.T) {
	type TestStruct struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Email string `json:"email,omitempty"`
	}

	req := &Request{}

	err := WithSchema(TestStruct{})(req)
	assert.NoError(t, err)
	assert.NotNil(t, req.ResponseFormat)
	assert.Equal(t, "json_schema", req.ResponseFormat.Type)
	assert.NotNil(t, req.ResponseFormat.JSONSchema)
	assert.Equal(t, "TestStruct", req.ResponseFormat.JSONSchema.Name)
	assert.True(t, req.ResponseFormat.JSONSchema.Strict)

	// Verify schema structure
	var schema map[string]interface{}
	err = json.Unmarshal(req.ResponseFormat.JSONSchema.Schema, &schema)
	assert.NoError(t, err)
	assert.Equal(t, "object", schema["type"])

	properties, ok := schema["properties"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, properties, "name")
	assert.Contains(t, properties, "age")
	assert.Contains(t, properties, "email")
}

func TestWithSchema_WithJSONRawMessage(t *testing.T) {
	schemaJSON := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	req := &Request{}

	err := WithSchema(schemaJSON)(req)
	assert.NoError(t, err)
	assert.NotNil(t, req.ResponseFormat)
	assert.Equal(t, "json_schema", req.ResponseFormat.Type)
	assert.NotNil(t, req.ResponseFormat.JSONSchema)
	assert.Equal(t, "response", req.ResponseFormat.JSONSchema.Name)
	assert.Equal(t, schemaJSON, req.ResponseFormat.JSONSchema.Schema)
}

func TestWithSchema_WithString(t *testing.T) {
	schemaStr := `{"type":"object","properties":{"value":{"type":"number"}}}`
	req := &Request{}

	err := WithSchema(schemaStr)(req)
	assert.NoError(t, err)
	assert.NotNil(t, req.ResponseFormat)
	assert.Equal(t, "json_schema", req.ResponseFormat.Type)
	assert.NotNil(t, req.ResponseFormat.JSONSchema)
}

func TestWithSchema_WithByteSlice(t *testing.T) {
	schemaBytes := []byte(`{"type":"object","properties":{"id":{"type":"string"}}}`)
	req := &Request{}

	err := WithSchema(schemaBytes)(req)
	assert.NoError(t, err)
	assert.NotNil(t, req.ResponseFormat)
	assert.Equal(t, "json_schema", req.ResponseFormat.Type)
	assert.NotNil(t, req.ResponseFormat.JSONSchema)
}

func TestWithSchema_InvalidType(t *testing.T) {
	req := &Request{}

	// WithSchema expects struct, json.RawMessage, string, or []byte
	// Passing an int should fail
	err := WithSchema(42)(req)
	assert.Error(t, err)
}

func TestStructToJSONSchema(t *testing.T) {
	type User struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Email    string `json:"email,omitempty"`
		Optional string `json:"optional,omitempty"`
	}

	schema, name, err := structToJSONSchema(User{})
	assert.NoError(t, err)
	assert.Equal(t, "User", name)
	assert.Equal(t, "object", schema["type"])

	properties, ok := schema["properties"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, properties, "id")
	assert.Contains(t, properties, "name")
	assert.Contains(t, properties, "email")
	assert.Contains(t, properties, "optional")

	required, ok := schema["required"].([]string)
	assert.True(t, ok)
	// email and optional should not be in required (omitempty)
	assert.Contains(t, required, "id")
	assert.Contains(t, required, "name")
	assert.NotContains(t, required, "email")
	assert.NotContains(t, required, "optional")
}

func TestStructToJSONSchema_WithPointer(t *testing.T) {
	type TestStruct struct {
		Value string `json:"value"`
	}

	ptr := &TestStruct{}
	schema, name, err := structToJSONSchema(ptr)
	assert.NoError(t, err)
	assert.Equal(t, "TestStruct", name)
	assert.Equal(t, "object", schema["type"])
}

func TestStructToJSONSchema_InvalidType(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
	}{
		{"string", "not a struct"},
		{"int", 42},
		{"slice", []string{"a", "b"}},
		{"map", map[string]string{"key": "value"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := structToJSONSchema(tt.val)
			assert.Error(t, err)
		})
	}
}

func TestGoTypeToJSONType(t *testing.T) {
	tests := []struct {
		name     string
		goType   interface{}
		expected string
	}{
		{"string", "test", "string"},
		{"int", 42, "integer"},
		{"int8", int8(8), "integer"},
		{"int16", int16(16), "integer"},
		{"int32", int32(32), "integer"},
		{"int64", int64(64), "integer"},
		{"uint", uint(1), "integer"},
		{"uint8", uint8(8), "integer"},
		{"uint16", uint16(16), "integer"},
		{"uint32", uint32(32), "integer"},
		{"uint64", uint64(64), "integer"},
		{"float32", float32(3.14), "number"},
		{"float64", float64(3.14), "number"},
		{"bool", true, "boolean"},
		{"slice", []string{}, "array"},
		{"array", [3]int{}, "array"},
		{"map", map[string]int{}, "object"},
		{"struct", struct{}{}, "object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use reflection to get the type
			typ := reflect.TypeOf(tt.goType)
			result := goTypeToJSONType(typ)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGoTypeToJSONType_WithPointer(t *testing.T) {
	var strPtr *string
	typ := reflect.TypeOf(strPtr)
	result := goTypeToJSONType(typ)
	assert.Equal(t, "string", result)
}

func TestMultipleOptions(t *testing.T) {
	req := &Request{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	err := WithSystem("You are helpful")(req)
	assert.NoError(t, err)

	err = WithModel("gpt-4")(req)
	assert.NoError(t, err)

	err = WithTemperature(0.8)(req)
	assert.NoError(t, err)

	err = WithMaxTokens(1000)(req)
	assert.NoError(t, err)

	assert.Len(t, req.Messages, 2)
	assert.Equal(t, "gpt-4", req.Model)
	assert.NotNil(t, req.Temperature)
	assert.Equal(t, 0.8, *req.Temperature)
	assert.NotNil(t, req.MaxTokens)
	assert.Equal(t, 1000, *req.MaxTokens)
}
