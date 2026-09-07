import AppKit

struct InvocationWindow {
    let id: UInt32
    let pid: Int32
}

// The list is in front-to-back order. A background terminal or detached tmux
// client is not evidence that its window invoked this review.
func selectInvocationWindow(_ windows: [InvocationWindow], processIDs: [Int32], frontmostPID: Int32?) -> UInt32? {
    guard let pid = frontmostPID, processIDs.contains(pid) else { return nil }
    return windows.first(where: { $0.pid == pid })?.id
}

func selectRecordedInvocationWindow(_ windows: [InvocationWindow], windowID: UInt32, processIDs: [Int32]) -> UInt32? {
    windows.first(where: { $0.id == windowID && processIDs.contains($0.pid) })?.id
}

func foregroundInvocationWindow(processIDs: [Int32]) -> UInt32? {
    guard let window = foregroundTerminalWindow() else { return nil }
    return selectInvocationWindow([window], processIDs: processIDs, frontmostPID: window.pid)
}

func invokingPaneMatches(_ output: String, pane: String, clientPIDs: [Int]) -> Bool {
    guard !pane.isEmpty && !clientPIDs.isEmpty else { return false }
    var found = Set<Int>()
    for line in output.split(separator: "\n") {
        let fields = line.split(separator: "\t", omittingEmptySubsequences: false)
        guard fields.count == 2, let pid = Int(fields[0]), clientPIDs.contains(pid) else { continue }
        guard fields[1] == pane else { return false }
        found.insert(pid)
    }
    return found == Set(clientPIDs)
}

func verifyInvokingPane(_ pane: String, clientPIDs: [Int], socket: String) -> Bool {
    guard !pane.isEmpty && !clientPIDs.isEmpty && !socket.isEmpty,
          let executable = ["/opt/homebrew/bin/tmux", "/usr/local/bin/tmux", "/usr/bin/tmux"].first(where: { FileManager.default.isExecutableFile(atPath: $0) }) else { return false }
    let process = Process(), pipe = Pipe(), finished = DispatchSemaphore(value: 0)
    process.executableURL = URL(fileURLWithPath: executable)
    process.arguments = ["-S", socket, "list-clients", "-F", "#{client_pid}\t#{pane_id}"]
    process.standardOutput = pipe; process.standardError = FileHandle.nullDevice
    process.terminationHandler = { _ in finished.signal() }
    do { try process.run() } catch { return false }
    guard finished.wait(timeout: .now() + 1) == .success else {
        if process.isRunning { process.terminate() }
        return false
    }
    guard process.terminationStatus == 0 else { return false }
    let data = pipe.fileHandleForReading.readDataToEndOfFile()
    return invokingPaneMatches(String(decoding: data, as: UTF8.self), pane: pane, clientPIDs: clientPIDs)
}

func foregroundTerminalWindow() -> InvocationWindow? {
    guard let app = NSWorkspace.shared.frontmostApplication,
          terminalBundleIDs.contains(app.bundleIdentifier ?? "") else { return nil }
    let info = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] ?? []
    let windows = info.compactMap { row -> InvocationWindow? in
        guard row[kCGWindowLayer as String] as? Int == 0,
              let pid = row[kCGWindowOwnerPID as String] as? Int32,
              let id = row[kCGWindowNumber as String] as? UInt32 else { return nil }
        return InvocationWindow(id: id, pid: pid)
    }
    return windows.first(where: { $0.pid == app.processIdentifier })
}

private let terminalBundleIDs: Set<String> = [
    "com.apple.Terminal", "com.googlecode.iterm2", "com.mitchellh.ghostty",
    "com.raphaelamorim.rio", "org.alacritty", "net.kovidgoyal.kitty",
    "com.github.wez.wezterm", "dev.warp.Warp-Stable", "co.zeit.hyper"
]
