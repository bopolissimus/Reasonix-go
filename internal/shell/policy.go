package shell

// Policy controls which shell constructs are allowed and how they're checked.
type Policy struct {
	// AllowPipe permits pipeline segments (cmd1 | cmd2) if each segment
	// individually passes IsSafeCommand.
	AllowPipe bool

	// AllowInputRedir permits < and << (input/heredoc) redirections.
	AllowInputRedir bool

	// AllowSubshell permits (...) subshells. The subshell body is recursively
	// analyzed against this same Policy.
	AllowSubshell bool

	// MaxDepth limits AST recursion depth for nested constructs ($(…), (…)).
	// Zero means no limit. Exceeding this depth blocks the command.
	MaxDepth int

	// IsSafeCommand checks whether a single command (name + args) is safe to
	// run as a standalone invocation. The command name is the resolved argv[0]
	// after alias/function lookup (not done by this package — callers should
	// resolve before calling Analyze if they track aliases).
	IsSafeCommand func(argv []string) bool
}

// DefaultPolicy returns a Policy suitable for plan-mode pipeline checking:
// pipelines allowed, input redirections allowed, subshells allowed up to
// depth 10. IsSafeCommand must be set by the caller (it depends on the
// environment's safe-command list).
func DefaultPolicy() *Policy {
	return &Policy{
		AllowPipe:        true,
		AllowInputRedir:  true,
		AllowSubshell:    true,
		MaxDepth:         10,
	}
}
