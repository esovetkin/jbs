package eval

import (
	"fmt"

	"jbs/internal/ast"
	"jbs/internal/diag"
)

func EvalExpr(expr ast.Expr, env map[string]Value, diags *diag.Diagnostics) Value {
	if expr == nil {
		return Null()
	}
	switch e := expr.(type) {
	case ast.IdentExpr:
		if v, ok := env[e.Name]; ok {
			return v
		}
		diags.AddError("E100", fmt.Sprintf("unknown variable '%s'", e.Name), e.Span, "import or define the variable before use")
		return Null()
	case ast.QualifiedIdentExpr:
		key := e.Namespace + "." + e.Name
		if v, ok := env[key]; ok {
			return v
		}
		diags.AddError("E100", fmt.Sprintf("unknown variable '%s'", key), e.Span, "import or define the variable before use")
		return Null()
	case ast.StringExpr:
		return String(e.Value)
	case ast.NumberExpr:
		if e.Int {
			return Int(e.IntValue)
		}
		return Float(e.FloatValue)
	case ast.BoolExpr:
		return Bool(e.Value)
	case ast.ListExpr:
		items := make([]Value, 0, len(e.Items))
		for _, it := range e.Items {
			items = append(items, EvalExpr(it, env, diags))
		}
		return List(items)
	case ast.TupleExpr:
		items := make([]Value, 0, len(e.Items))
		for _, it := range e.Items {
			items = append(items, EvalExpr(it, env, diags))
		}
		return List(items)
	case ast.UnaryExpr:
		v := EvalExpr(e.Expr, env, diags)
		return evalUnary(e.Op, v, e.Span, diags)
	case ast.BinaryExpr:
		l := EvalExpr(e.Left, env, diags)
		r := EvalExpr(e.Right, env, diags)
		return evalBinary(e.Op, l, r, e.Span, diags)
	case ast.CompareExpr:
		l := EvalExpr(e.Left, env, diags)
		r := EvalExpr(e.Right, env, diags)
		return evalCompare(e.Op, l, r, e.Span, diags)
	case ast.ConditionalExpr:
		c := EvalExpr(e.Cond, env, diags)
		if c.Kind != KindBool {
			diags.AddError("E102", "conditional requires boolean condition", e.Cond.GetSpan(), "ensure condition evaluates to true/false")
			return EvalExpr(e.Then, env, diags)
		}
		if c.B {
			return EvalExpr(e.Then, env, diags)
		}
		return EvalExpr(e.Else, env, diags)
	case ast.ModeExpr:
		return EvalExpr(e.Expr, env, diags)
	default:
		diags.AddError("E199", "unsupported expression node", expr.GetSpan(), "check expression syntax")
		return Null()
	}
}

func evalUnary(op string, v Value, at diag.Span, diags *diag.Diagnostics) Value {
	if v.Kind == KindList {
		out := make([]Value, len(v.L))
		for i, it := range v.L {
			out[i] = evalUnary(op, it, at, diags)
		}
		return List(out)
	}
	if !isNumeric(v) {
		diags.AddError("E103", fmt.Sprintf("unary '%s' requires numeric value", op), at, "use int/float values")
		return Null()
	}
	if op == "+" {
		return v
	}
	if v.Kind == KindFloat {
		return Float(-v.F)
	}
	return Int(-v.I)
}

func evalBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	if op == "and" || op == "or" {
		if l.Kind != KindBool || r.Kind != KindBool {
			diags.AddError("E104", fmt.Sprintf("'%s' requires boolean operands", op), at, "use boolean values with and/or")
			return Null()
		}
		if op == "and" {
			return Bool(l.B && r.B)
		}
		return Bool(l.B || r.B)
	}

	if l.Kind == KindList || r.Kind == KindList {
		return evalVectorBinary(op, l, r, at, diags)
	}
	if l.Kind == KindString || r.Kind == KindString {
		if op != "+" {
			diags.AddError("E105", fmt.Sprintf("operator '%s' is not supported for strings", op), at, "use '+' for string concatenation")
			return Null()
		}
		return String(l.String() + r.String())
	}
	if !isNumeric(l) || !isNumeric(r) {
		diags.AddError("E106", fmt.Sprintf("operator '%s' requires numeric or string operands", op), at, "check operand types")
		return Null()
	}

	lf := toFloat(l)
	rf := toFloat(r)
	switch op {
	case "+":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf + rf)
		}
		return Int(l.I + r.I)
	case "-":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf - rf)
		}
		return Int(l.I - r.I)
	case "*":
		if l.Kind == KindFloat || r.Kind == KindFloat {
			return Float(lf * rf)
		}
		return Int(l.I * r.I)
	case "/":
		if rf == 0 {
			diags.AddError("E107", "division by zero", at, "guard denominator")
			return Null()
		}
		return Float(lf / rf)
	case "%":
		if r.Kind == KindFloat || l.Kind == KindFloat {
			diags.AddError("E108", "modulo requires integer operands", at, "use int values with '%' operator")
			return Null()
		}
		if r.I == 0 {
			diags.AddError("E107", "modulo by zero", at, "guard denominator")
			return Null()
		}
		return Int(l.I % r.I)
	default:
		diags.AddError("E109", fmt.Sprintf("unknown operator '%s'", op), at, "use supported operators")
		return Null()
	}
}

func evalVectorBinary(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	ls := ToSeries(l)
	rs := ToSeries(r)
	if len(ls) == 0 || len(rs) == 0 {
		return List(nil)
	}
	n := len(ls)
	if len(rs) > n {
		n = len(rs)
	}
	out := make([]Value, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, evalBinary(op, ls[i%len(ls)], rs[i%len(rs)], at, diags))
	}
	return List(out)
}

func evalCompare(op string, l, r Value, at diag.Span, diags *diag.Diagnostics) Value {
	switch op {
	case "==":
		return Bool(Equal(l, r))
	case "!=":
		return Bool(!Equal(l, r))
	}

	if l.Kind == KindString && r.Kind == KindString {
		switch op {
		case "<":
			return Bool(l.S < r.S)
		case "<=":
			return Bool(l.S <= r.S)
		case ">":
			return Bool(l.S > r.S)
		case ">=":
			return Bool(l.S >= r.S)
		}
	}
	if isNumeric(l) && isNumeric(r) {
		lf := toFloat(l)
		rf := toFloat(r)
		switch op {
		case "<":
			return Bool(lf < rf)
		case "<=":
			return Bool(lf <= rf)
		case ">":
			return Bool(lf > rf)
		case ">=":
			return Bool(lf >= rf)
		}
	}
	diags.AddError("E110", fmt.Sprintf("unsupported comparison '%s' for operand types", op), at, "compare compatible types")
	return Bool(false)
}
