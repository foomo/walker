import Foundation

/// SettingsMigration moves the settings of the shape developer from the keys
/// they had while shapes were called schemas - at launch, before anything
/// reads them. A value already stored under the new key wins.
enum SettingsMigration {
    static func run(_ defaults: UserDefaults = .standard) {
        for (key, value) in defaults.dictionaryRepresentation() {
            guard let new = renamed(key) else { continue }
            if defaults.object(forKey: new) == nil {
                defaults.set(value, forKey: new)
            }
            defaults.removeObject(forKey: key)
        }
        // the content switch of the outline is gone - a branch shows its content on a click
        defaults.removeObject(forKey: "outlineShowsContent")
    }

    /// renamed is the current name of an old key - schemaDevEditor is
    /// shapeDevEditor, schemaDev.<server>.schemaRoot is shapeDev.<server>.shapeRoot -
    /// nil for every other key
    static func renamed(_ key: String) -> String? {
        let old = "schemaDev"
        guard key.hasPrefix(old) else { return nil }
        var new = "shapeDev" + key.dropFirst(old.count)
        if new.hasPrefix("shapeDev."), new.hasSuffix(".schemaRoot") {
            new = String(new.dropLast("schemaRoot".count)) + "shapeRoot"
        }
        return new
    }
}
