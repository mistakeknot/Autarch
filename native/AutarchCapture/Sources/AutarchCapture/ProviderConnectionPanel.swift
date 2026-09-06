import AppKit

// A separate companion window keeps provider URLs and device codes out of the
// selected Autarch TUI window. CaptureModel excludes all of our process's windows
// from both its recording and screenshot sources. Nothing here uses the outbox.
@MainActor final class ProviderConnectionPanel: NSObject {
    static let shared = ProviderConnectionPanel()
    private var task: Task<Void, Never>?
    private var panel: NSPanel?
    private let instructions = NSTextView()
    private let input = NSSecureTextField()
    private let options = NSPopUpButton()
    private let submit = NSButton(title: "Submit response", target: nil, action: nil)
    private let cancel = NSButton(title: "Cancel sign-in", target: nil, action: nil)
    private var state: [String: Any] = [:]
    private var promptKey = ""
    private var shownOperation = ""
    private var submitting = false

    func start() {
        guard task == nil else { return }
        task = Task {
            while !Task.isCancelled {
                let response = try? await Task.detached { try IPC.call(["method": "auth.display"]) }.value
                update(response?["auth"] as? [String: Any] ?? [:])
                try? await Task.sleep(for: .seconds(1))
            }
        }
    }
    func update(_ next: [String: Any]) {
        state = next
        guard let operation = next["operation"] as? [String: Any], operation["status"] as? String == "pending" else {
            input.stringValue = ""; instructions.string = ""; options.removeAllItems()
            state = [:]; promptKey = ""; shownOperation = ""; panel?.orderOut(nil)
            return
        }
        makePanel()
        let operationID = operation["id"] as? String ?? ""
        let displayID = (next["displayId"] as? String ?? "") + "/" + operationID
        if shownOperation != displayID {
            shownOperation = displayID; panel?.makeKeyAndOrderFront(nil); NSApp.activate(ignoringOtherApps: true)
        }
        let prompt = operation["prompt"] as? [String: Any]
        let key = (next["runtimeId"] as? String ?? "") + "/" + operationID + "/" + (prompt?["id"] as? String ?? "")
        if key != promptKey { input.stringValue = ""; promptKey = key; options.removeAllItems() }
        var text = "Provider sign-in\n\n"
        for event in operation["events"] as? [[String: Any]] ?? [] {
            for field in ["message", "url", "instructions", "verificationUri", "userCode"] {
                if let value = event[field] as? String, !value.isEmpty { text += value + "\n" }
            }
            for link in event["links"] as? [[String: Any]] ?? [] {
                if let url = link["url"] as? String { text += url + "\n" }
            }
        }
        text += "\n" + (prompt?["message"] as? String ?? "Complete sign-in in your browser.")
        let choices = prompt?["options"] as? [[String: Any]] ?? []
        if options.numberOfItems != choices.count {
            options.removeAllItems()
            options.addItems(withTitles: choices.map { $0["label"] as? String ?? "Option" })
        }
        let attributed = NSMutableAttributedString(string: text, attributes: [.font: NSFont.systemFont(ofSize: 14)])
        if let detector = try? NSDataDetector(types: NSTextCheckingResult.CheckingType.link.rawValue) {
            for match in detector.matches(in: text, range: NSRange(text.startIndex..., in: text)) {
                if let url = match.url, ["https", "http"].contains(url.scheme ?? "") { attributed.addAttribute(.link, value: url, range: match.range) }
            }
        }
        instructions.textStorage?.setAttributedString(attributed)
        input.isHidden = prompt == nil || !choices.isEmpty
        options.isHidden = choices.isEmpty
        submit.isEnabled = prompt != nil && !submitting
        cancel.isEnabled = !submitting
    }
    private func makePanel() {
        guard panel == nil else { return }
        let window = NSPanel(contentRect: NSRect(x: 0, y: 0, width: 680, height: 420), styleMask: [.titled, .closable, .resizable], backing: .buffered, defer: false)
        window.title = "Autarch · Connect provider"
        window.isReleasedWhenClosed = false
        instructions.isEditable = false; instructions.isSelectable = true
        let scroll = NSScrollView(); scroll.documentView = instructions; scroll.hasVerticalScroller = true
        instructions.autoresizingMask = [.width]; instructions.textContainer?.widthTracksTextView = true
        scroll.heightAnchor.constraint(greaterThanOrEqualToConstant: 280).isActive = true
        submit.target = self; submit.action = #selector(respond)
        cancel.target = self; cancel.action = #selector(cancelLogin)
        let buttons = NSStackView(views: [submit, cancel]); buttons.orientation = .horizontal
        let stack = NSStackView(views: [scroll, input, options, buttons]); stack.orientation = .vertical; stack.alignment = .leading; stack.spacing = 12
        stack.edgeInsets = NSEdgeInsets(top: 16, left: 16, bottom: 16, right: 16)
        window.contentView = stack
        for view in [scroll, input, options] { view.widthAnchor.constraint(equalTo: stack.widthAnchor, constant: -32).isActive = true }
        window.center(); panel = window
    }
    @objc private func respond() { send(cancelled: false) }
    @objc private func cancelLogin() { send(cancelled: true) }
    private func send(cancelled: Bool) {
        guard !submitting, let operation = state["operation"] as? [String: Any] else { return }
        let prompt = operation["prompt"] as? [String: Any]
        var auth: [String: Any] = ["runtimeId": state["runtimeId"] ?? "", "operationId": operation["id"] ?? ""]
        if !cancelled {
            guard let prompt else { return }
            auth["promptId"] = prompt["id"]
            let choices = prompt["options"] as? [[String: Any]] ?? []
            auth["value"] = choices.isEmpty ? input.stringValue : choices[max(0, options.indexOfSelectedItem)]["id"]
        }
        input.stringValue = ""
        submitting = true; submit.isEnabled = false; cancel.isEnabled = false
        let request: [String: Any] = ["method": cancelled ? "auth.cancel" : "auth.respond", "project": state["project"] ?? "", "auth": auth]
        Task {
            let result = try? await Task.detached { try IPC.call(request) }.value
            submitting = false
            if let next = result?["auth"] as? [String: Any] { update(next) }
        }
    }
}
