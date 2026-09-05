import SwiftUI
import AVKit

@main struct AutarchCaptureApp: App {
    @StateObject private var capture = CaptureModel()
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
                    Button("Refresh windows") { capture.perform { try await capture.loadWindows() } }
                    Button(capture.paused ? "Resume" : "Start recording") { capture.perform { try await capture.start() } }.disabled(capture.recording || capture.selectedWindow == 0 || capture.project.isEmpty)
                    Button("Pause") { capture.perform { try await capture.stop(pausing: true) } }.disabled(!capture.recording)
                    Button("Stop") { capture.perform { try await capture.stop(pausing: false) } }.disabled(!capture.recording && !capture.paused)
                }
                if let image = capture.preview { Image(nsImage: image).resizable().scaledToFit().frame(maxHeight: 260) }
                if let player = capture.player { VideoPlayer(player: player).frame(minHeight: 200) }
                Text("Quick feedback · ⌘⇧Space brings this window forward").font(.headline)
                TextEditor(text: $capture.note).frame(height: 90).border(.secondary.opacity(0.3))
                HStack {
                    Button("Save note + screenshot") { capture.perform { try await capture.saveNote() } }.keyboardShortcut("s", modifiers: [.command])
                    Button(capture.voiceActive ? "Finish voice note" : "Record voice note") { capture.perform { try await capture.toggleVoice() } }
                    Button("Play session") { capture.playSession() }.disabled(capture.media.isEmpty)
                    Spacer(); Text(capture.usage).font(.caption)
                }
                Text(capture.status).foregroundStyle(capture.failed ? .red : .secondary).textSelection(.enabled)
                Text("Full sessions and original voice notes stay on this Mac until you delete them. Closing Autarch does not stop recording.").font(.caption)
            }.padding(20).frame(minWidth: 680, minHeight: 450).task { await capture.run() }
        }
        .commands { CommandGroup(replacing: .newItem) {} }
    }
}
