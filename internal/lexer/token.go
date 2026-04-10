package lexer

import "jbs/internal/diag"

type TokenType string

const (
	TokenEOF          TokenType = "EOF"
	TokenNewline      TokenType = "NEWLINE"
	TokenIdent        TokenType = "IDENT"
	TokenNumber       TokenType = "NUMBER"
	TokenString       TokenType = "STRING"
	TokenComma        TokenType = ","
	TokenSemicolon    TokenType = ";"
	TokenDot          TokenType = "."
	TokenEqual        TokenType = "="
	TokenPlusEqual    TokenType = "+="
	TokenMinusEqual   TokenType = "-="
	TokenStarEqual    TokenType = "*="
	TokenSlashEqual   TokenType = "/="
	TokenPercentEqual TokenType = "%="
	TokenLParen       TokenType = "("
	TokenRParen       TokenType = ")"
	TokenLBracket     TokenType = "["
	TokenRBracket     TokenType = "]"
	TokenLBrace       TokenType = "{"
	TokenRBrace       TokenType = "}"
	TokenPlus         TokenType = "+"
	TokenMinus        TokenType = "-"
	TokenStar         TokenType = "*"
	TokenSlash        TokenType = "/"
	TokenPercent      TokenType = "%"
	TokenEqEq         TokenType = "=="
	TokenNeq          TokenType = "!="
	TokenLT           TokenType = "<"
	TokenGT           TokenType = ">"
	TokenLE           TokenType = "<="
	TokenGE           TokenType = ">="
	TokenIf           TokenType = "if"
	TokenElse         TokenType = "else"
	TokenAnd          TokenType = "and"
	TokenOr           TokenType = "or"
	TokenParam        TokenType = "param"
	TokenDo           TokenType = "do"
	TokenSubmit       TokenType = "submit"
	TokenLet          TokenType = "let"
	TokenAnalyse      TokenType = "analyse"
	TokenWith         TokenType = "with"
	TokenFrom         TokenType = "from"
	TokenAfter        TokenType = "after"
	TokenIn           TokenType = "in"
	TokenAs           TokenType = "as"
	TokenUse          TokenType = "use"
)

type Token struct {
	Type  TokenType
	Text  string
	Value string
	Span  diag.Span
}

func (t Token) IsKeyword(tt TokenType) bool {
	return t.Type == tt
}

var keywords = map[string]TokenType{
	"if":      TokenIf,
	"else":    TokenElse,
	"and":     TokenAnd,
	"or":      TokenOr,
	"param":   TokenParam,
	"do":      TokenDo,
	"submit":  TokenSubmit,
	"let":     TokenLet,
	"analyse": TokenAnalyse,
	"with":    TokenWith,
	"from":    TokenFrom,
	"after":   TokenAfter,
	"in":      TokenIn,
	"as":      TokenAs,
	"use":     TokenUse,
}
