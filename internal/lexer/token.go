// define JBS keywords, literals, operators, statement separators
package lexer

import "gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"

type TokenType string

const (
	TokenEOF          TokenType = "EOF"
	TokenNewline      TokenType = "NEWLINE"
	TokenComment      TokenType = "COMMENT"
	TokenIdent        TokenType = "IDENT"
	TokenNumber       TokenType = "NUMBER"
	TokenString       TokenType = "STRING"
	TokenComma        TokenType = ","
	TokenColon        TokenType = ":"
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
	TokenBang         TokenType = "!"
	TokenAmp          TokenType = "&"
	TokenPipe         TokenType = "|"
	TokenEqEq         TokenType = "=="
	TokenNeq          TokenType = "!="
	TokenLT           TokenType = "<"
	TokenGT           TokenType = ">"
	TokenLE           TokenType = "<="
	TokenGE           TokenType = ">="
	TokenIf           TokenType = "if"
	TokenElif         TokenType = "elif"
	TokenElse         TokenType = "else"
	TokenFor          TokenType = "for"
	TokenWhile        TokenType = "while"
	TokenBreak        TokenType = "break"
	TokenContinue     TokenType = "continue"
	TokenAnd          TokenType = "and"
	TokenOr           TokenType = "or"
	TokenFunction     TokenType = "function"
	TokenReturn       TokenType = "return"
	TokenDo           TokenType = "do"
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
	"if":       TokenIf,
	"elif":     TokenElif,
	"else":     TokenElse,
	"for":      TokenFor,
	"while":    TokenWhile,
	"break":    TokenBreak,
	"continue": TokenContinue,
	"and":      TokenAnd,
	"or":       TokenOr,
	"function": TokenFunction,
	"return":   TokenReturn,
	"do":       TokenDo,
	"analyse":  TokenAnalyse,
	"with":     TokenWith,
	"from":     TokenFrom,
	"after":    TokenAfter,
	"in":       TokenIn,
	"as":       TokenAs,
	"use":      TokenUse,
}
