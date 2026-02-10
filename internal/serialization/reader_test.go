package serialization

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubstituteEnvReader_Basic(t *testing.T) {
	t.Setenv("TEST_VAR", "hello")

	input := []byte(`key: ${TEST_VAR}`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, `key: "hello"`, string(output))
}

func TestSubstituteEnvReader_Multiple(t *testing.T) {
	t.Setenv("VAR1", "first")
	t.Setenv("VAR2", "second")

	input := []byte(`a: ${VAR1}, b: ${VAR2}`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, `a: "first", b: "second"`, string(output))
}

func TestSubstituteEnvReader_NoSubstitution(t *testing.T) {
	input := []byte(`key: value`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, `key: value`, string(output))
}

func TestSubstituteEnvReader_UnsetEnvError(t *testing.T) {
	input := []byte(`key: ${UNSET_VAR_FOR_TEST}`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	_, err := io.ReadAll(reader)
	require.Error(t, err)
	require.Contains(t, err.Error(), "UNSET_VAR_FOR_TEST is not set")
}

func TestSubstituteEnvReader_SmallBuffer(t *testing.T) {
	t.Setenv("SMALL_BUF_VAR", "value")

	input := []byte(`key: ${SMALL_BUF_VAR}`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	var result []byte
	buf := make([]byte, 3)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
	}
	require.Equal(t, `key: "value"`, string(result))
}

func TestSubstituteEnvReader_SpecialChars(t *testing.T) {
	t.Setenv("SPECIAL_VAR", `hello "world" \n`)

	input := []byte(`key: ${SPECIAL_VAR}`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, `key: "hello \"world\" \\n"`, string(output))
}

func TestSubstituteEnvReader_EmptyValue(t *testing.T) {
	t.Setenv("EMPTY_VAR", "")

	input := []byte(`key: ${EMPTY_VAR}`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, `key: ""`, string(output))
}

func TestSubstituteEnvReader_LargeInput(t *testing.T) {
	t.Setenv("LARGE_VAR", "replaced")

	prefix := strings.Repeat("x", 5000)
	suffix := strings.Repeat("y", 5000)
	input := []byte(prefix + "${LARGE_VAR}" + suffix)

	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	expected := prefix + `"replaced"` + suffix
	require.Equal(t, expected, string(output))
}

func TestSubstituteEnvReader_PatternAtBoundary(t *testing.T) {
	t.Setenv("BOUNDARY_VAR", "boundary_value")

	prefix := strings.Repeat("a", 4090)
	input := []byte(prefix + "${BOUNDARY_VAR}")

	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	expected := prefix + `"boundary_value"`
	require.Equal(t, expected, string(output))
}

func TestSubstituteEnvReader_MultiplePatternsBoundary(t *testing.T) {
	t.Setenv("VAR_A", "aaa")
	t.Setenv("VAR_B", "bbb")

	prefix := strings.Repeat("x", 4090)
	input := []byte(prefix + "${VAR_A} middle ${VAR_B}")

	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	expected := prefix + `"aaa" middle "bbb"`
	require.Equal(t, expected, string(output))
}

func TestSubstituteEnvReader_YAMLConfig(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_PASSWORD", "secret123")

	input := []byte(`database:
  host: ${DB_HOST}
  port: ${DB_PORT}
  password: ${DB_PASSWORD}
`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	expected := `database:
  host: "localhost"
  port: "5432"
  password: "secret123"
`
	require.Equal(t, expected, string(output))
}

func TestSubstituteEnvReader_DollarWithoutBrace(t *testing.T) {
	input := []byte(`key: $NOT_A_PATTERN`)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, `key: $NOT_A_PATTERN`, string(output))
}

func TestSubstituteEnvReader_EmptyInput(t *testing.T) {
	input := []byte(``)
	reader := NewSubstituteEnvReader(bytes.NewReader(input))

	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, ``, string(output))
}

func TestFindIncompletePatternStart(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"no pattern", "hello world", -1},
		{"complete pattern", "hello ${VAR} world", -1},
		{"dollar at end", "hello $", 6},
		{"dollar brace at end", "hello ${", 6},
		{"incomplete var at end", "hello ${VAR", 6},
		{"complete then incomplete", "hello ${VAR} ${INCOMPLETE", 13},
		{"multiple complete", "${A} ${B} ${C}", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findIncompletePatternStart([]byte(tt.input))
			require.Equal(t, tt.expected, result)
		})
	}
}
