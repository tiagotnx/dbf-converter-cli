package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilter_NilExpressionAlwaysMatches(t *testing.T) {
	f, err := New("")
	require.NoError(t, err)
	assert.Nil(t, f, "empty expression should return nil filter for fast path")
}

func TestFilter_StringEquality(t *testing.T) {
	f, err := New("STATUS == 'A'")
	require.NoError(t, err)

	match, err := f.Match(map[string]interface{}{"STATUS": "A", "VALOR": 100.0})
	require.NoError(t, err)
	assert.True(t, match)

	match, err = f.Match(map[string]interface{}{"STATUS": "B", "VALOR": 100.0})
	require.NoError(t, err)
	assert.False(t, match)
}

func TestFilter_NumericComparison(t *testing.T) {
	f, err := New("VALOR >= 150")
	require.NoError(t, err)

	tests := []struct {
		valor float64
		want  bool
	}{
		{100.0, false},
		{150.0, true},
		{999.99, true},
		{0.0, false},
	}
	for _, tt := range tests {
		got, err := f.Match(map[string]interface{}{"VALOR": tt.valor})
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, "VALOR=%v", tt.valor)
	}
}

func TestFilter_LogicalAnd(t *testing.T) {
	f, err := New("STATUS == 'A' && VALOR >= 150")
	require.NoError(t, err)

	tests := []struct {
		env  map[string]interface{}
		want bool
	}{
		{map[string]interface{}{"STATUS": "A", "VALOR": 200.0}, true},
		{map[string]interface{}{"STATUS": "A", "VALOR": 100.0}, false},
		{map[string]interface{}{"STATUS": "B", "VALOR": 999.0}, false},
		{map[string]interface{}{"STATUS": "A", "VALOR": 150.0}, true},
	}
	for _, tt := range tests {
		got, err := f.Match(tt.env)
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, "env=%v", tt.env)
	}
}

func TestFilter_InvalidExpression(t *testing.T) {
	_, err := New("VALOR >>> 'broken'")
	assert.Error(t, err, "malformed expression must fail at compile time")
}

func TestFilter_NonBooleanResultIsError(t *testing.T) {
	f, err := New("VALOR + 1")
	require.NoError(t, err)

	_, err = f.Match(map[string]interface{}{"VALOR": 10.0})
	assert.Error(t, err, "non-boolean filter result must be an error")
}

func TestFilter_HandlesNilValues(t *testing.T) {
	// Filter expressions should tolerate nil values (empty numerics, empty dates).
	f, err := New("VALOR != nil && VALOR > 0")
	require.NoError(t, err)

	match, err := f.Match(map[string]interface{}{"VALOR": nil})
	require.NoError(t, err)
	assert.False(t, match)

	match, err = f.Match(map[string]interface{}{"VALOR": 10.0})
	require.NoError(t, err)
	assert.True(t, match)
}
