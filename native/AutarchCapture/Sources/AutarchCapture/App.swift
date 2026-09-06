import SwiftUI
import AVKit

@main struct AutarchCaptureApp: App {
    @StateObject private var capture = CaptureModel()
    @StateObject private var shortcut = CaptureShortcutController()
    @State private var showingShortcut = false
    var body: some Scene {
        WindowGroup("Autarch Review Companion") {
            VStack(alignment: .leading, spacing: 12) {
                HStack { Text("Autarch Review").font(.title); Spacer(); Text(capture.recording ? "● Recording selected window" : capture.paused ? "Paused" : "Ready").foregroundStyle(capture.recording ? .red : .secondary) }
                Text(capture.project.isEmpty ? "Open a project review in Autarch to choose the destination." : capture.project).font(.caption).textSelection(.enabled)
                Picker("Test window", selection: $capture.selectedWindow) {
                    Text("Select a window").tag(UInt32(0))
                    ForEach(capture.windows, id: \.windowID) { window in Text("\(window.owningApplication?.applicationName ?? "App") — \(window.title ?? "Untitled")").tag(window.windowID) }
                }.disabled(capture.recording || capture.paused)
                HStack {
                    if capture.capturePermissionNeeded { Button("Screen recording settings") { capture.openCaptureSettings() } }
                    Button("Refresh windows") { capture.perform { try await capture.loadWindows() } }
                    Button(capture.paused ? "Resume" : "Start recording") { capture.perform { try await capture.start() } }.disabled(capture.recording || capture.selectedWindow == 0 || capture.project.isEmpty)
                    Button("Pause") { capture.perform { try await capture.stop(pausing: true) } }.disabled(!capture.recording)
                    Button("Stop") { capture.perform { try await capture.stop(pausing: false) } }.disabled(!capture.recording && !capture.paused)
                }
                if let image = capture.preview { Image(nsImage: image).resizable().scaledToFit().frame(maxHeight: 260) }
                if let player = capture.player { ReviewPlayerView(player: player).frame(minHeight: 200) }
                HStack {
                    Picker("Retained session", selection: $capture.selectedRetainedSession) {
                        Text("Select a session").tag("")
                        ForEach(capture.retainedSessions) { saved in Text(saved.title).tag(saved.id) }
                    }
                    Button("Play selected session") { capture.playRetainedSession() }.disabled(capture.selectedRetainedSession.isEmpty)
                }
                HStack {
                    Text("Quick feedback · \(shortcut.active?.label ?? "Shortcut disabled")").font(.headline)
                    Spacer()
                    Button("Change shortcut…") { showingShortcut = true }
                }
                if !shortcut.error.isEmpty { Text(shortcut.error).foregroundStyle(.red) }
                TextEditor(text: $capture.note).frame(height: 90).border(.secondary.opacity(0.3))
                HStack {
                    Button("Save note + screenshot") { capture.perform { try await capture.saveNote() } }.keyboardShortcut("s", modifiers: [.command])
                    Button(capture.voiceActive ? "Finish voice note" : "Record voice note") { capture.perform { try await capture.toggleVoice() } }
                    Button("Play session") { capture.playSession() }.disabled(capture.media.isEmpty)
                    Spacer(); Text(capture.usage).font(.caption)
                }
                Text(capture.status).foregroundStyle(capture.failed ? .red : .secondary).textSelection(.enabled)
                Text("Full sessions and original voice notes stay on this Mac until you delete them. Closing Autarch does not stop recording.").font(.caption)
            }.padding(20).frame(minWidth: 680, minHeight: 450)
            .sheet(isPresented: $showingShortcut) {
                VStack {
                    CaptureShortcutSettings(shortcut: shortcut)
                    Button("Done") { showingShortcut = false }.keyboardShortcut(.cancelAction).padding(.bottom, 16)
                }
            }
            .task {
                shortcut.start { Task { await capture.quickMoment() } }
                if CommandLine.arguments.contains("--configure-shortcut") { showingShortcut = true }
                ProviderConnectionPanel.shared.start()
                await capture.run()
            }
        }
        .commands { CommandGroup(replacing: .newItem) {} }
        Settings { CaptureShortcutSettings(shortcut: shortcut) }
    }
}

// Use AppKit's concrete player view; the macOS 26 SwiftUI VideoPlayer bridge
// failed during superclass metadata initialization in the live pilot.
struct ReviewPlayerView: NSViewRepresentable {
    let player: AVPlayer
    func makeNSView(context: Context) -> AVPlayerView {
        let view = AVPlayerView()
        view.controlsStyle = .inline
        view.player = player
        return view
    }
    func updateNSView(_ view: AVPlayerView, context: Context) {
        if view.player !== player { view.player = player }
    }
}
