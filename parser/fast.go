package parser

import (
	"errors"

	"github.com/ZxxLang/zxx/scanner"
	"github.com/ZxxLang/zxx/token"
)

// Symbol
type Symbol struct {
	Pos    scanner.Pos
	Tok    token.Token
	Source string
}

// Fast 快速解析, 转换, 合并 zxx 源码 src 中的 Token.
//
// 参数 rec 用于逐个接收解析到的 Token, 包括 EOF.
// 如果 rec 为 nil, 返回值 nodes 包含所有的 Token, 不包括 EOF.
//
// 缺陷:
//
// Fast 通过分析源码缩进判断顶层占位, 这可能对英文(非多字节)开始的顶层占位有影响.
// 常规的缩进或用 '//', '---' 开始英文顶层占位可以弥补缺陷.
//
func Fast(src []byte, cb func(scanner.Pos, token.Token, string) error) (nodes []Symbol, err error) {
	var eml, indent string
	var delay, tok, prev token.Token

	if cb == nil {
		nodes = make([]Symbol, 0, len(src)/10)
	}

	rec := func(pos scanner.Pos, tok token.Token, code string) (err error) {
		// 合并空白行和占位为 PLACEHOLDER
		switch tok {
		case token.NL:
			if delay == token.EOF {
				delay = token.PLACEHOLDER
				break
			}
			if prev == tok {
				eml += code
				if delay == token.EOF {
					delay = token.NL
				} else {
					delay = token.PLACEHOLDER
				}
				return
			}

			if prev == token.INDENTATION {
				eml += code
				delay = token.PLACEHOLDER
				return
			}

			break
		case token.PLACEHOLDER, token.COMMENT:
			if prev == token.INDENTATION {
				eml += indent
				indent = ""
			}
			eml += code
			delay = token.PLACEHOLDER
			return
		case token.INDENTATION:
			indent = code
			return
		}

		if eml != "" {
			if cb == nil {
				nodes = append(nodes, Symbol{pos.Offset(-len(eml)), token.PLACEHOLDER, eml})
			} else {
				err = cb(pos.Offset(-len(eml)), token.PLACEHOLDER, eml)
				if err != nil {
					return
				}
			}
			eml = ""
		}

		if prev == token.INDENTATION {
			if cb == nil {
				nodes = append(nodes, Symbol{pos.Offset(-len(indent)), token.INDENTATION, indent})
			} else {
				err = cb(pos.Offset(-len(indent)), token.INDENTATION, indent)
				if err != nil {
					return
				}
			}
			indent = ""
		}

		if cb == nil {
			nodes = append(nodes, Symbol{pos, tok, code})
		} else {
			err = cb(pos, tok, code)
		}
		return
	}

	tabKind := false
	isTop := true
	scan := scanner.New(src)

	for err == nil {

		pos := scan.Pos()
		code, ok := scan.Symbol()
		//fmt.Println(pos, code, token.Lookup(code))
		if !ok {
			err = errors.New("invalid UTF-8 encode")
			return
		}

		prev = tok
		tok = token.Lookup(code)

		if tok == token.EOF {
			if nodes == nil {
				err = rec(pos, tok, code)
			}
			return
		}

		if isTop {
			isTop = false
			if !tok.As(token.Declare) {
				var tmp string
				posi := pos
				for ok && tok != token.EOF && !tok.As(token.Declare) {
					code += scan.Tail(true) + tmp
					pos = scan.Pos()
					tmp, ok = scan.Symbol()
					tok = token.Lookup(tmp)
				}

				if !ok {
					err = errors.New("invalid UTF-8 encode")
					return
				}
				err = rec(posi, token.PLACEHOLDER, code)
				code = tmp
			}
			if err == nil {
				err = rec(pos, tok, code)
			}
			continue
		}

		switch tok {

		case token.SPACES:
			// 不支持 SPACES, TABS 混搭缩进
			if prev == token.INDENTATION ||
				tabKind && prev == token.NL {
				err = errors.New("parser: bad indentation style for TABS + SPACES")
				return
			}
			if prev == token.NL {
				tok = token.INDENTATION
				break
			}
			// 丢弃分隔空格
			continue

		case token.TABS:
			if prev == token.INDENTATION {
				err = errors.New("parser: bad indentation style for SPACES + TABS")
				return
			}
			if prev == token.NL {
				tok = token.INDENTATION
				tabKind = true
			} else {
				// TABS 尾注释
				code += scan.Tail(false)
				tok = token.COMMENT
			}
		case token.COMMENT:
			err = rec(pos, tok, code+scan.Tail(false))
			continue
		case token.COMMENTS:
			// 完整块注释
			for {
				tmp, _ := scan.Symbol()
				code += tmp
				tok = token.Lookup(tmp)
				if tok == token.COMMENTS || tok == token.EOF {
					break
				}
			}
			if tok != token.COMMENTS {
				err = errors.New("parser: COMMENTS is incomplete")
				return
			}
			err = rec(pos, tok, code+scan.Tail(false))
			continue
		case token.TRUE, token.FALSE:
			tok = token.VALBOOL
		case token.NAN, token.INFINITE:
			tok = token.VALFLOAT
		case token.PLACEHOLDER:
			// 识别语义, 只剩下字面值和标识符, 成员
			if code == `"` || code == `'` {
				// 完整字符串
				code += scan.EndString(code == `"`)
				if code[0] != code[len(code)-1] {
					err = errors.New("parser: string is incomplete")
					return
				}
				tok = token.VALSTRING
				break
			}
			// 整数, 浮点数, datetime
			// ??? 缺少严格检查
			if code[0] >= '0' && code[0] <= '9' {
				tok = token.VALINTEGER
				if code[0] == '0' && len(code) > 2 && (code[1] == 'x' || code[1] == 'b') {
				} else {
					for _, c := range code {
						if c == '.' || c == 'e' {
							tok = token.VALFLOAT
						} else if c == 'T' || c == ':' || c == 'Z' {
							tok = token.VALDATETIME
						} else if (c < '0' || c > '9') && c != '+' && c != '-' && c != '_' {
							tok = token.PLACEHOLDER
							break
						}
					}
				}
			} else {
				// 标识符, 成员
				tok = token.IDENT
				dot := 0
				for _, c := range code {
					if c == '.' {
						dot++
						continue
					}

					if c != '_' && !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
						tok = token.PLACEHOLDER
						break
					}
				}
				if dot != 0 && tok == token.IDENT {
					if dot == 1 {
						tok = token.MEMBER
					} else {
						tok = token.MEMBERS
					}
				}
			}
		}
		err = rec(pos, tok, code)
	}
	return
}
