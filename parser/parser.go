// Copyright 2016 The Zxx Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"errors"

	"github.com/ZxxLang/zxx/ast"
	"github.com/ZxxLang/zxx/scanner"
	"github.com/ZxxLang/zxx/token"
)

func symbolIfy(s string, ok bool) string {
	return s
}

// Parse 解析, 转换, 合并 zxx 源码 src 中的 Token 到 ast.File.
//
// 转化细节:
//
// 	COMMENT     替代 COMMENTS
// 	VALFLOAT    替代 NAN, INFINITE
// 	VALBOOL     替代 TRUE, FALSE
// 	INDENTATION 替代行首的 SPACES, TABS
// 	忽略 Token 之间 SPACES

//	干净的源码没有多余的占位和注释, 解析过程就是选取干净的 Token 构成当前节点.
//	缩进, 占位, 注释,间隔符号, 分号, 换行只是被保存, 永远不会成为当前节点.
//	逗号, 分号, 换行用于产生 FFinal 标记, 并切换当前节点.
//
func Parse(src []byte, file *ast.File) (err error) {
	var (
		tabKind bool // 缩进风格
	)

	scan := scanner.New(src)
	for err == nil && !scan.IsEOF() {
		pos := scan.Pos()
		code, ok := scan.Symbol()

		if !ok {
			err = errors.New("invalid UTF-8 encode")
			break
		}

		tok := token.Lookup(code)

		// 根节点, 只包含声明和占位, 非声明都转换为占位
		if file.Active == file {
			if !tok.As(token.Declare) {
				// 占位扫描
				var tmp string
				for ok && tok != token.EOF && !tok.As(token.Declare) {
					code += scan.Tail(true) + tmp
					pos = scan.Pos()
					tmp, ok = scan.Symbol()
					tok = token.Lookup(tmp)
				}
				if !ok {
					err = errors.New("invalid UTF-8 encode")
					break
				}

				if err = file.Push(pos, token.PLACEHOLDER, code); err != nil {
					break
				}
				code = tmp
			}
			err = file.Push(pos, tok, code)
			continue
		}

		last := file.Last
		// 脏 Token 全部由 File 解决, 并且不影响当前节点
		//
		switch tok {

		case token.SPACES:
			// 不支持 SPACES, TABS 混搭缩进
			if last.Token() == token.INDENTATION ||
				tabKind && last.Token() == token.NL {
				err = errors.New("parser: bad indentation style for TABS + SPACES")
				continue
			}
			if last.Token() == token.NL {
				tok = token.INDENTATION
				break
			}
			// 丢弃分隔空格
			continue

		case token.TABS:
			if last.Token() == token.INDENTATION {
				err = errors.New("parser: bad indentation style for SPACES + TABS")
				continue
			}
			if last.Token() == token.NL {
				tok = token.INDENTATION
				tabKind = true
			} else {
				// TABS 尾注释
				code += scan.Tail(false)
				tok = token.COMMENT
			}
		case token.COMMENT:
			err = file.Push(pos, tok, code+scan.Tail(false))
			continue
		case token.COMMENTS:
			// 完整块注释
			for !scan.IsEOF() {
				tmp, _ := scan.Symbol()
				code += tmp
				tok = token.Lookup(tmp)
				if tok == token.COMMENTS {
					break
				}
			}
			if tok != token.COMMENTS {
				err = errors.New("parser: COMMENTS is incomplete")
			} else {
				err = file.Push(pos, tok, code+scan.Tail(false))
			}
			continue
		case token.DOT: // MEMBER, SUGAR
		case token.TRUE, token.FALSE:
			tok = token.VALBOOL
		case token.NAN, token.INFINITE:
			tok = token.VALFLOAT
		// case token.NULL:
		case token.PLACEHOLDER:
			// 识别语义, 只剩下字面值和标识符, 成员
			if code == "\"" || code == "'" {
				// 完整字符串
				code += scan.EndString(code == "\"")
				if scan.IsEOF() {
					err = errors.New("parser: string is incomplete")
					continue
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

		if err == nil {
			err = file.Push(pos, tok, code)
		}
	}
	return
}
