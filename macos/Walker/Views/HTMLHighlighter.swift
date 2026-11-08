import SwiftUI

/// HTMLHighlighter colors html for display - tag names, attribute names and
/// values, comments - in the colors of JSONHighlighter. It never fails:
/// broken html is colored as far as it can be read.
enum HTMLHighlighter {
    static func highlight(_ html: String) -> AttributedString {
        var result = AttributedString()
        let chars = Array(html)
        var i = 0

        func append(_ text: some StringProtocol, _ color: Color, italic: Bool = false) {
            var part = AttributedString(String(text))
            part.foregroundColor = color
            if italic {
                part.inlinePresentationIntent = .emphasized
            }
            result += part
        }

        func starts(with prefix: String, at index: Int) -> Bool {
            let p = Array(prefix)
            return index + p.count <= chars.count && Array(chars[index..<index + p.count]) == p
        }

        while i < chars.count {
            if starts(with: "<!--", at: i) {
                var j = i + 4
                while j < chars.count, !starts(with: "-->", at: j) {
                    j += 1
                }
                j = min(j + 3, chars.count)
                append(chars[i..<j].map(String.init).joined(), .secondary, italic: true)
                i = j
            } else if chars[i] == "<" {
                // the tag: < or </, the name, the attributes, > or />
                var j = i + 1
                if j < chars.count, chars[j] == "/" {
                    j += 1
                }
                append(String(chars[i..<j]), .secondary)
                var k = j
                while k < chars.count, !chars[k].isWhitespace, chars[k] != ">", chars[k] != "/" {
                    k += 1
                }
                append(String(chars[j..<k]), .purple)
                i = k
                while i < chars.count, chars[i] != ">" {
                    let c = chars[i]
                    if c.isWhitespace || c == "/" {
                        append(String(c), .secondary)
                        i += 1
                    } else if c == "=" {
                        append("=", .secondary)
                        i += 1
                        if i < chars.count, chars[i] == "\"" || chars[i] == "'" {
                            let quote = chars[i]
                            var end = i + 1
                            while end < chars.count, chars[end] != quote {
                                end += 1
                            }
                            end = min(end + 1, chars.count)
                            append(String(chars[i..<end]), .red)
                            i = end
                        }
                    } else {
                        var end = i
                        while end < chars.count, !chars[end].isWhitespace, chars[end] != "=", chars[end] != ">", chars[end] != "/" {
                            end += 1
                        }
                        append(String(chars[i..<end]), .blue)
                        i = end
                    }
                }
                if i < chars.count {
                    append(">", .secondary)
                    i += 1
                }
            } else {
                var j = i
                while j < chars.count, chars[j] != "<" {
                    j += 1
                }
                append(String(chars[i..<j]), .primary)
                i = j
            }
        }
        return result
    }
}
