package shell

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// SegmentDecision records the decision for a single command segment.
type SegmentDecision struct {
	Text    string
	Allowed bool
	Reason  string
}

// Result from analyzing a shell command.
type Result struct {
	Allowed  bool
	Message  string
	Segments []SegmentDecision
}

// Analyze walks the parsed AST and classifies every segment against the
// provided safety policy. Fail-closed: unrecognized AST nodes, parse errors,
// or policy violations all result in Allowed=false.
func Analyze(f *syntax.File, policy *Policy) Result {
	if f == nil {
		return Result{Allowed: false, Message: "no AST"}
	}
	w := &walker{policy: policy}
	for _, stmt := range f.Stmts {
		w.walkStmt(stmt, "")
	}
	return w.result()
}

type walker struct {
	policy    *Policy
	depth     int
	decisions []SegmentDecision
}

func (w *walker) add(text string, allowed bool, reason string) {
	w.decisions = append(w.decisions, SegmentDecision{
		Text:    text,
		Allowed: allowed,
		Reason:  reason,
	})
}

func (w *walker) result() Result {
	allowed := true
	for _, d := range w.decisions {
		if !d.Allowed {
			allowed = false
			break
		}
	}
	msg := "ok"
	if !allowed {
		var reasons []string
		for _, d := range w.decisions {
			if !d.Allowed {
				reasons = append(reasons, d.Reason)
			}
		}
		msg = strings.Join(reasons, "; ")
	}
	return Result{Allowed: allowed, Message: msg, Segments: w.decisions}
}

func (w *walker) walkStmt(stmt *syntax.Stmt, separator string) {
	if stmt == nil {
		return
	}

	// Check depth
	w.depth++
	if w.policy.MaxDepth > 0 && w.depth > w.policy.MaxDepth {
		w.add("<depth exceeded>", false,
			fmt.Sprintf("max AST depth %d exceeded", w.policy.MaxDepth))
		w.depth--
		return
	}

	// Check background execution
	if stmt.Background {
		w.add("&", false, "background execution is not allowed")
	}

	// Check redirections
	for _, r := range stmt.Redirs {
		w.walkRedirect(r)
	}

	// Walk the command
	if stmt.Cmd != nil {
		w.walkCmd(stmt.Cmd)
	}

	w.depth--
}

func (w *walker) walkCmd(cmd syntax.Command) {
	switch c := cmd.(type) {
	case *syntax.CallExpr:
		w.walkCallExpr(c)
	case *syntax.BinaryCmd:
		w.walkBinaryCmd(c)
	case *syntax.Subshell:
		w.walkSubshell(c)
	case *syntax.Block:
		w.walkBlock(c)
	case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause,
		*syntax.CaseClause, *syntax.FuncDecl, *syntax.DeclClause,
		*syntax.TestClause, *syntax.TimeClause, *syntax.CoprocClause,
		*syntax.LetClause:
		w.add(fmt.Sprintf("<%T>", cmd), false,
			fmt.Sprintf("unsupported shell construct: %T", cmd))
	default:
		w.add(fmt.Sprintf("<%T>", cmd), false,
			fmt.Sprintf("unsupported shell construct: %T", cmd))
	}
}

func (w *walker) walkCallExpr(ce *syntax.CallExpr) {
	argv := callExprArgv(ce)
	if w.policy.IsSafeCommand != nil && !w.policy.IsSafeCommand(argv) {
		w.add(strings.Join(argv, " "), false,
			fmt.Sprintf("command %q is not in the safe list", argv[0]))
	} else {
		w.add(strings.Join(argv, " "), true, "safe command")
	}
}

func (w *walker) walkBinaryCmd(bc *syntax.BinaryCmd) {
	// BinaryCmd covers: | (Pipe), && (AndStmt), || (OrStmt), |& (PipeAll)
	switch bc.Op {
	case syntax.Pipe, syntax.PipeAll:
		if !w.policy.AllowPipe {
			w.add("|", false, "pipelines are not allowed")
			return
		}
		// Walk both sides — pipe segments are independent (no state leakage).
		w.walkStmt(bc.X, "|")
		w.walkStmt(bc.Y, "")
	case syntax.AndStmt:
		w.add("&&", false,
			"conditional execution (&&) is not yet supported — use separate commands")
	case syntax.OrStmt:
		w.add("||", false,
			"conditional execution (||) is not yet supported — use separate commands")
	default:
		w.add(fmt.Sprintf("op(%v)", bc.Op), false,
			fmt.Sprintf("unknown binary operator: %v", bc.Op))
	}
}

func (w *walker) walkSubshell(ss *syntax.Subshell) {
	if !w.policy.AllowSubshell {
		w.add("(...)", false, "subshells are not allowed")
		return
	}
	for _, stmt := range ss.Stmts {
		w.walkStmt(stmt, "")
	}
}

func (w *walker) walkBlock(bl *syntax.Block) {
	if !w.policy.AllowSubshell {
		w.add("{...}", false, "command blocks are not allowed")
		return
	}
	for _, stmt := range bl.Stmts {
		w.walkStmt(stmt, "")
	}
}

func (w *walker) walkRedirect(r *syntax.Redirect) {
	switch r.Op {
	case syntax.RdrIn, syntax.Hdoc, syntax.RdrInOut, syntax.DplIn:
		// Input redirections: <, <<, <>, <&
		if w.policy.AllowInputRedir {
			w.add(redirName(r.Op), true, "input redirection")
		} else {
			w.add(redirName(r.Op), false, "input redirections are not allowed")
		}
	default:
		// Output redirections: >, >>, >&, >|, &>, etc.
		w.add(redirName(r.Op), false,
			fmt.Sprintf("output redirection (%s) is not allowed", redirName(r.Op)))
	}
}

func redirName(op syntax.RedirOperator) string {
	switch op {
	case syntax.RdrIn:
		return "<"
	case syntax.RdrOut:
		return ">"
	case syntax.AppOut:
		return ">>"
	case syntax.Hdoc:
		return "<<"
	case syntax.RdrInOut:
		return "<>"
	case syntax.DplIn:
		return "<&"
	case syntax.DplOut:
		return ">&"
	case syntax.RdrClob:
		return ">|"
	default:
		return fmt.Sprintf("redir(%d)", op)
	}
}

// callExprArgv reconstructs argv from a CallExpr's Args slice.
func callExprArgv(ce *syntax.CallExpr) []string {
	if len(ce.Args) == 0 {
		return nil
	}
	argv := make([]string, len(ce.Args))
	for i, w := range ce.Args {
		argv[i] = wordLiteral(w)
	}
	return argv
}

// wordLiteral extracts the approximate literal text from a Word.
func wordLiteral(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		switch part := p.(type) {
		case *syntax.Lit:
			b.WriteString(part.Value)
		case *syntax.SglQuoted:
			b.WriteString("'" + part.Value + "'")
		case *syntax.DblQuoted:
			b.WriteString(`"` + wordPartsLiteral(part.Parts) + `"`)
		default:
			b.WriteString("$")
		}
	}
	return b.String()
}

// wordPartsLiteral extracts literal text from a slice of WordParts.
func wordPartsLiteral(parts []syntax.WordPart) string {
	var b strings.Builder
	for _, p := range parts {
		switch part := p.(type) {
		case *syntax.Lit:
			b.WriteString(part.Value)
		case *syntax.SglQuoted:
			b.WriteString("'" + part.Value + "'")
		case *syntax.DblQuoted:
			b.WriteString(`"` + wordPartsLiteral(part.Parts) + `"`)
		default:
			b.WriteString("$")
		}
	}
	return b.String()
}
