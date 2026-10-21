import Foundation

/// PromSample is one sample of the Prometheus text exposition format:
/// `walker_requests_total{group="product",status="200"} 42`
struct PromSample: Hashable, Sendable {
    let name: String
    let labels: [String: String]
    let value: Double
}

/// PromText parses what a /metrics endpoint serves - comments, HELP and TYPE
/// lines are skipped, timestamps are ignored
enum PromText {
    static func parse(_ text: String) -> [PromSample] {
        var samples: [PromSample] = []
        for line in text.split(separator: "\n", omittingEmptySubsequences: true) {
            if line.hasPrefix("#") { continue }
            if let sample = sample(line) {
                samples.append(sample)
            }
        }
        return samples
    }

    private static func sample(_ line: Substring) -> PromSample? {
        guard let nameEnd = line.firstIndex(where: { $0 == "{" || $0 == " " }) else { return nil }
        let name = String(line[..<nameEnd])
        var rest = line[nameEnd...]
        var labels: [String: String] = [:]
        if rest.first == "{" {
            rest = rest.dropFirst()
            while let first = rest.first, first != "}" {
                if first == "," || first == " " {
                    rest = rest.dropFirst()
                    continue
                }
                guard let equals = rest.firstIndex(of: "="),
                      rest.index(after: equals) < rest.endIndex,
                      rest[rest.index(after: equals)] == "\""
                else { return nil }
                let key = String(rest[..<equals])
                rest = rest[rest.index(equals, offsetBy: 2)...]
                var value = ""
                var closed = false
                while let character = rest.first {
                    rest = rest.dropFirst()
                    if character == "\"" {
                        closed = true
                        break
                    }
                    if character == "\\", let escaped = rest.first {
                        rest = rest.dropFirst()
                        value.append(escaped == "n" ? "\n" : escaped)
                    } else {
                        value.append(character)
                    }
                }
                guard closed else { return nil }
                labels[key] = value
            }
            guard rest.first == "}" else { return nil }
            rest = rest.dropFirst()
        }
        // the value, then an optional timestamp
        guard let token = rest.split(separator: " ", omittingEmptySubsequences: true).first,
              let value = number(token)
        else { return nil }
        return PromSample(name: name, labels: labels, value: value)
    }

    private static func number(_ token: Substring) -> Double? {
        switch token {
        case "+Inf", "Inf": .infinity
        case "-Inf": -.infinity
        case "NaN": .nan
        default: Double(token)
        }
    }
}
