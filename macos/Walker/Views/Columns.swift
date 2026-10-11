import CoreGraphics

/// Columns keeps the column widths of the window in one place - the minimums
/// of all visible columns must fit next to each other, or AppKit breaks
/// constraints.
enum Columns {
    struct ColumnWidth {
        let min: CGFloat
        let ideal: CGFloat
        let max: CGFloat
    }

    static let serverSidebar = ColumnWidth(min: 180, ideal: 220, max: 320)
    static let resultsSidebar = ColumnWidth(min: 180, ideal: 240, max: 360)
    static let resultsTableMinWidth: CGFloat = 320
    /// the inspector starts wide - with the preview it needs the room more than the table
    static let inspector = ColumnWidth(min: 640, ideal: 880, max: 1400)
}
