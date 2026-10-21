import SwiftUI

/// JSONHighlighter colors JSON for display - keys, JSON-LD keywords, strings,
/// numbers and literals. It never fails: broken JSON is colored as far as it
/// can be read.
enum JSONHighlighter {
    static func highlight(_ json: String) -> AttributedString {
        var result = AttributedString()
        let chars = Array(json)
        var i = 0

        func append(_ text: String, _ color: Color, bold: Bool = false, underline: Bool = false) {
            var part = AttributedString(text)
            part.foregroundColor = color
            if bold {
                part.inlinePresentationIntent = .stronglyEmphasized
            }
            if underline {
                part.underlineStyle = .single
            }
            result += part
        }

        /// isKey looks past the string for the colon of an object key
        func isKey(after end: Int) -> Bool {
            var j = end
            while j < chars.count, chars[j].isWhitespace {
                j += 1
            }
            return j < chars.count && chars[j] == ":"
        }

        while i < chars.count {
            let c = chars[i]
            switch c {
            case "\"":
                var j = i + 1
                while j < chars.count, chars[j] != "\"" {
                    j += chars[j] == "\\" ? 2 : 1
                }
                let end = min(j + 1, chars.count)
                let text = String(chars[i..<end])
                if isKey(after: end) {
                    if text.hasPrefix("\"@") {
                        append(text, .purple, bold: true)
                    } else {
                        append(text, .blue)
                    }
                } else {
                    let isURL = text.hasPrefix("\"http://") || text.hasPrefix("\"https://")
                    append(text, .red, underline: isURL)
                }
                i = end
            case "-", "0"..."9":
                var j = i + 1
                while j < chars.count, chars[j].isNumber || "+-.eE".contains(chars[j]) {
                    j += 1
                }
                append(String(chars[i..<j]), .teal)
                i = j
            case "t", "f", "n":
                var j = i
                while j < chars.count, chars[j].isLetter {
                    j += 1
                }
                let word = String(chars[i..<j])
                append(word, ["true", "false", "null"].contains(word) ? .orange : .primary)
                i = max(j, i + 1)
            case "{", "}", "[", "]", ",", ":":
                append(String(c), .secondary)
                i += 1
            default:
                // whitespace and whatever broken JSON contains
                var j = i + 1
                while j < chars.count, !"\"{}[],:-0123456789tfn".contains(chars[j]) {
                    j += 1
                }
                append(String(chars[i..<j]), .primary)
                i = j
            }
        }
        return result
    }
}

/// JSONMark marks parts of a JSON-LD document: the objects of a @type, or a
/// property path inside them - arrays fan out like in the validator
struct JSONMark: Equatable {
    /// locator of the script the document is in
    let locator: String
    let type: String
    /// path below the objects of type, empty marks the objects themselves
    let path: [String]
    let color: Color
    /// missing marks the objects a required property is missing from
    let missing: Bool
}

extension JSONHighlighter {
    /// highlight colors the JSON and puts a background behind the marked parts
    static func highlight(_ json: String, mark: JSONMark?) -> AttributedString {
        var result = highlight(json)
        guard let mark else { return result }
        let chars = Array(json)
        var parser = JSONParser(chars: chars)
        guard let root = parser.parse() else { return result }
        var ranges: [Range<Int>] = []
        for object in root.objects(ofType: mark.type) {
            ranges += mark.path.isEmpty ? [object.range] : object.ranges(of: mark.path[...])
        }
        let background = mark.color.opacity(mark.missing ? 0.12 : 0.28)
        for range in ranges where range.upperBound <= chars.count {
            let start = result.characters.index(result.startIndex, offsetBy: range.lowerBound)
            let end = result.characters.index(result.startIndex, offsetBy: range.upperBound)
            result[start..<end].backgroundColor = background
        }
        return result
    }
}

/// JSONNode is a parsed JSON value with its position in the text
private final class JSONNode {
    enum Kind { case object, array, string, other }

    let kind: Kind
    var range: Range<Int>
    /// members of an object: key, offset of the key, value
    var members: [(key: String, keyStart: Int, value: JSONNode)] = []
    var items: [JSONNode] = []
    var string = ""

    init(kind: Kind, range: Range<Int>) {
        self.kind = kind
        self.range = range
    }

    func member(_ key: String) -> JSONNode? {
        members.first { $0.key == key }?.value
    }

    /// objects of a JSON-LD @type in this value, @graph included
    func objects(ofType type: String) -> [JSONNode] {
        switch kind {
        case .array:
            return items.flatMap { $0.objects(ofType: type) }
        case .object:
            var found: [JSONNode] = []
            if let t = member("@type"), t.string == type || t.items.contains(where: { $0.string == type }) {
                found.append(self)
            }
            if let graph = member("@graph") {
                found += graph.objects(ofType: type)
            }
            return found
        default:
            return []
        }
    }

    /// ranges of a property path - from the key of the last step to the end of its value
    func ranges(of path: ArraySlice<String>) -> [Range<Int>] {
        guard let step = path.first else { return [] }
        switch kind {
        case .array:
            return items.flatMap { $0.ranges(of: path) }
        case .object:
            guard let member = members.first(where: { $0.key == step }) else { return [] }
            if path.count == 1 {
                return [member.keyStart..<member.value.range.upperBound]
            }
            return member.value.ranges(of: path.dropFirst())
        default:
            return []
        }
    }
}

/// JSONParser is a small recursive descent parser that keeps positions - nil for broken JSON
private struct JSONParser {
    let chars: [Character]
    var i = 0

    mutating func parse() -> JSONNode? {
        let value = parseValue()
        skipWhitespace()
        return i == chars.count ? value : nil
    }

    private mutating func skipWhitespace() {
        while i < chars.count, chars[i].isWhitespace {
            i += 1
        }
    }

    private mutating func parseValue() -> JSONNode? {
        skipWhitespace()
        guard i < chars.count else { return nil }
        switch chars[i] {
        case "{": return parseObject()
        case "[": return parseArray()
        case "\"": return parseString()
        default:
            let start = i
            while i < chars.count, !",]} \n\r\t".contains(chars[i]) {
                i += 1
            }
            return i > start ? JSONNode(kind: .other, range: start..<i) : nil
        }
    }

    private mutating func parseString() -> JSONNode? {
        let start = i
        i += 1
        var value = ""
        while i < chars.count, chars[i] != "\"" {
            if chars[i] == "\\", i + 1 < chars.count {
                i += 1
            }
            value.append(chars[i])
            i += 1
        }
        guard i < chars.count else { return nil }
        i += 1
        let node = JSONNode(kind: .string, range: start..<i)
        node.string = value
        return node
    }

    private mutating func parseObject() -> JSONNode? {
        let node = JSONNode(kind: .object, range: i..<i)
        i += 1
        skipWhitespace()
        if i < chars.count, chars[i] == "}" {
            i += 1
            node.range = node.range.lowerBound..<i
            return node
        }
        while i < chars.count {
            skipWhitespace()
            let keyStart = i
            guard i < chars.count, chars[i] == "\"", let key = parseString() else { return nil }
            skipWhitespace()
            guard i < chars.count, chars[i] == ":" else { return nil }
            i += 1
            guard let value = parseValue() else { return nil }
            node.members.append((key.string, keyStart, value))
            skipWhitespace()
            guard i < chars.count else { return nil }
            if chars[i] == "," {
                i += 1
                continue
            }
            guard chars[i] == "}" else { return nil }
            i += 1
            node.range = node.range.lowerBound..<i
            return node
        }
        return nil
    }

    private mutating func parseArray() -> JSONNode? {
        let node = JSONNode(kind: .array, range: i..<i)
        i += 1
        skipWhitespace()
        if i < chars.count, chars[i] == "]" {
            i += 1
            node.range = node.range.lowerBound..<i
            return node
        }
        while i < chars.count {
            guard let value = parseValue() else { return nil }
            node.items.append(value)
            skipWhitespace()
            guard i < chars.count else { return nil }
            if chars[i] == "," {
                i += 1
                continue
            }
            guard chars[i] == "]" else { return nil }
            i += 1
            node.range = node.range.lowerBound..<i
            return node
        }
        return nil
    }
}
