// Package filter wraps the expr-lang engine to compile a user-supplied --where
// expression once at startup and evaluate it against each streamed record.
package filter

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Filter holds a pre-compiled expression program ready to be evaluated per record.
// A nil *Filter is a valid "match-everything" sentinel and callers should treat it
// as the fast path.
type Filter struct {
	program *vm.Program
	source  string
}

// New compiles expression once. An empty/whitespace-only expression yields a nil filter
// (meaning "no filtering"); callers can short-circuit on the nil check for performance.
func New(expression string) (*Filter, error) {
	if strings.TrimSpace(expression) == "" {
		return nil, nil
	}
	program, err := expr.Compile(expression, expr.AllowUndefinedVariables())
	if err != nil {
		return nil, fmt.Errorf("compiling filter expression %q: %w", expression, err)
	}
	return &Filter{program: program, source: expression}, nil
}

// Match evaluates the compiled expression against env and returns its boolean result.
// Non-boolean results are rejected so that malformed filters are surfaced instead of silently
// matching/rejecting rows.
func (f *Filter) Match(env map[string]interface{}) (bool, error) {
	if f == nil {
		return true, nil
	}
	out, err := expr.Run(f.program, env)
	if err != nil {
		return false, fmt.Errorf("evaluating filter %q: %w", f.source, err)
	}
	b, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("filter expression must return bool, got %T (%v)", out, out)
	}
	return b, nil
}
