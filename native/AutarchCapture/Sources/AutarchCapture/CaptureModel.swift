import AppKit
import AVFoundation
import ScreenCaptureKit
import Speech
import Carbon

@MainActor final class CaptureModel: NSObject, ObservableObject {
    @Published var windows: [SCWindow] = []
    @Published var selectedWindow: UInt32 = 0
    @Published var project = ""
    @Published var note = ""
    @Published var status = "Select a test window, then start a review."
    @Published var usage = ""
    @Published var recording = false
    @Published var paused = false
    @Published var voiceActive = false
    @Published var failed = false
    @Published var preview: NSImage?
    @Published var player: AVPlayer?
    private(set) var media: [[String: Any]] = []
    private var session: [String: Any] = [:]
    private var stream: SCStream?
    private var output: SCRecordingOutput?
    private var recorderDelegate: RecordingDelegate?
    private var audio: AVAudioRecorder?
    private var voiceURL: URL?
    private var voiceStart: Int64 = 0
    private var sessionStart = Date()
    private var segmentStart = Date()
    private var started = false
    private var working = false
    private var hotKey: EventHotKeyRef?
    private var outbox: Outbox?
    private var editedFeedback: [String: Any]?
    private var uiContext: [String: Any] = [:]
    private var lastHeartbeat = Date.distantPast
    private var recovered = false
    private var sessionDir: URL { IPC.directory.appendingPathComponent("media/" + (session["id"] as? String ?? "intake")) }
    private var offset: Int64 { Int64(Date().timeIntervalSince(sessionStart) * 1000) }

    func perform(_ operation: @escaping @MainActor () async throws -> Void) {
        guard !working else { return }; working = true
        Task { defer { working = false }; do { try await operation(); failed = false } catch { status = error.localizedDescription; failed = true } }
    }
    func run() async {
        guard !started else { return }; started = true
        do { outbox = try Outbox(directory: IPC.directory.appendingPathComponent("capture-outbox")); installShortcut() }
        catch { status = error.localizedDescription; failed = true; return }
        while !Task.isCancelled {
            do {
                let box = outbox!
                let response = try await Task.detached { () throws -> [String: Any] in
                    for request in try box.pending() {
                        do { _ = try IPC.call(request); try box.acknowledge(request["id"] as! String) }
                        catch CaptureError.rejected(let reason) {
                            try box.retainRejected(request["id"] as! String)
                            throw CaptureError.message("Capture retained for recovery (\(reason)); see \(box.directory.path)")
                        }
                    }
                    return try IPC.call(["method": "state"])
                }.value
                if let state = response["state"] as? [String: Any] {
                    if let context = state["context"] as? [String: Any] { uiContext = context; if !recording && !paused { switchProject(context["project"] as? String ?? project) } }
                    if !recovered { try await recoverInterruptedSessions(); recovered = true }
                    if !working, let commands = state["commands"] as? [[String: Any]], let command = commands.first(where: { $0["status"] as? String == "pending" }) {
                        working = true
                        do { try await handle(command); try queue(["method": "capture.ack", "target": command["id"]!, "status": "done"]) }
                        catch { status = error.localizedDescription; failed = true; try queue(["method": "capture.ack", "target": command["id"]!, "status": "failed: " + error.localizedDescription]) }
                        working = false
                    }
                }
                let size = response["storage_bytes"] as? Int64 ?? 0
                usage = ByteCountFormatter.string(fromByteCount: size, countStyle: .file) + " retained locally"
                if recording && Date().timeIntervalSince(lastHeartbeat) > 5 { try persistSession("recording"); lastHeartbeat = Date() }
            } catch { status = error.localizedDescription; failed = true; working = false }
            try? await Task.sleep(for: .seconds(1))
        }
    }
    private func queue(_ body: [String: Any]) throws {
        guard let outbox else { throw CaptureError.message("Local storage unavailable") }
        var request = body; if request["project"] == nil { request["project"] = project }
        try outbox.enqueue(request)
    }
    private func persistSession(_ state: String) throws {
        session["status"] = state; session["media"] = media; session["project"] = project
        try queue(["method": "session.save", "session": session])
        session["revision"] = (session["revision"] as? Int ?? 0) + 1
        let data = try JSONSerialization.data(withJSONObject: session)
        try data.write(to: sessionDir.appendingPathComponent("capture-session.json"), options: .atomic)
    }
    private func switchProject(_ destination: String) {
        guard project != destination else { return }
        project = destination; session = [:]; media = []; editedFeedback = nil
        player = nil; preview = nil
    }
    private func recoverInterruptedSessions() async throws {
        let root = IPC.directory.appendingPathComponent("media")
        guard let folders = try? FileManager.default.contentsOfDirectory(at: root, includingPropertiesForKeys: nil) else { return }
        for folder in folders {
            let path = folder.appendingPathComponent("capture-session.json")
            guard let data = try? Data(contentsOf: path), var saved = try? JSONSerialization.jsonObject(with: data) as? [String: Any], let originalProject = saved["project"] as? String else { continue }
            guard ["recording", "starting"].contains(saved["status"] as? String ?? "") else { continue }
            var sources = saved["media"] as? [[String: Any]] ?? []
            for i in sources.indices where sources[i]["status"] as? String == "recording" {
                let asset = AVURLAsset(url: URL(fileURLWithPath: sources[i]["path"] as? String ?? ""))
                sources[i]["status"] = (try? await asset.load(.isPlayable)) == true ? "available" : "unavailable"
            }
            saved["media"] = sources; saved["status"] = "interrupted"; saved["error"] = "Capture process stopped unexpectedly; all original files retained."
            try queue(["method": "session.save", "project": originalProject, "session": saved])
            saved["revision"] = (saved["revision"] as? Int ?? 0) + 1
            try JSONSerialization.data(withJSONObject: saved).write(to: path, options: .atomic)
        }
    }
    func loadWindows() async throws {
        let content = try await SCShareableContent.excludingDesktopWindows(true, onScreenWindowsOnly: true)
        windows = content.windows.filter { $0.owningApplication?.processID != ProcessInfo.processInfo.processIdentifier && $0.frame.width > 80 && $0.frame.height > 80 }.sorted { ($0.title ?? "") < ($1.title ?? "") }
    }
    private func configuration(_ window: SCWindow) -> SCStreamConfiguration {
        let config = SCStreamConfiguration()
        let scale = min(2.0, 1920.0 / max(1, window.frame.width))
        config.width = max(2, Int(window.frame.width * scale) / 2 * 2)
        config.height = max(2, Int(window.frame.height * scale) / 2 * 2)
        config.minimumFrameInterval = CMTime(value: 1, timescale: 30)
        config.showsCursor = true; config.capturesAudio = false; config.captureMicrophone = false
        return config
    }
    func start() async throws {
        guard !recording, !project.isEmpty, let window = windows.first(where: { $0.windowID == selectedWindow }) else { throw CaptureError.message("Select the project and test window first") }
        if !paused {
            editedFeedback = nil
            sessionStart = Date(); media = []
            session = ["id": UUID().uuidString, "revision": 0, "window_id": selectedWindow, "window_title": "\(window.owningApplication?.applicationName ?? "App") — \(window.title ?? "Untitled")"]
        }
        try FileManager.default.createDirectory(at: sessionDir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let url = sessionDir.appendingPathComponent(UUID().uuidString + ".mp4")
        let configuration = SCRecordingOutputConfiguration(); configuration.outputURL = url
        let delegate = RecordingDelegate(); recorderDelegate = delegate
        let output = SCRecordingOutput(configuration: configuration, delegate: delegate); self.output = output
        let stream = SCStream(filter: SCContentFilter(desktopIndependentWindow: window), configuration: self.configuration(window), delegate: nil); self.stream = stream
        try stream.addRecordingOutput(output)
        segmentStart = Date()
        media.append(["id": UUID().uuidString, "path": url.path, "status": "recording", "kind": "video", "offset_ms": offset])
        try persistSession("starting")
        do { try await stream.startCapture(); recording = true; paused = false; try persistSession("recording"); status = "Recording only the selected window. Microphone is off." }
        catch { session["error"] = error.localizedDescription; try? persistSession("interrupted"); throw error }
    }
    func stop(pausing: Bool) async throws {
        guard recording || paused else { throw CaptureError.message("No active review recording") }
        if voiceActive { try await toggleVoice() }
        if let stream, recording {
            try await stream.stopCapture()
            if let delegate = recorderDelegate { try await delegate.waitForFinish() }
            if !media.isEmpty { media[media.count - 1]["status"] = "available" }
        }
        recording = false; paused = pausing; stream = nil; output = nil
        try persistSession(pausing ? "paused" : "stopped")
        status = pausing ? "Paused. Completed segments are retained." : "Complete session retained locally."
    }
    private func screenshot() async throws -> [String: Any]? {
        guard let window = windows.first(where: { $0.windowID == selectedWindow }) else { return nil }
        let image = try await SCScreenshotManager.captureImage(contentFilter: SCContentFilter(desktopIndependentWindow: window), configuration: configuration(window))
        preview = NSImage(cgImage: image, size: .zero)
        let representation = NSBitmapImageRep(cgImage: image)
        guard let data = representation.representation(using: .png, properties: [:]) else { throw CaptureError.message("Could not encode screenshot") }
        try FileManager.default.createDirectory(at: sessionDir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let id = UUID().uuidString; let url = sessionDir.appendingPathComponent(id + ".png")
        try data.write(to: url, options: .atomic)
        return ["id": id, "path": url.path, "status": "available", "kind": "screenshot", "offset_ms": offset]
    }
    func saveNote() async throws {
        guard !note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        var feedback = editedFeedback ?? ["id": UUID().uuidString, "revision": 0, "context": uiContext]
        feedback["text"] = note
        if session["id"] != nil { feedback["session_id"] = session["id"] }
        var evidence = feedback["evidence"] as? [[String: Any]] ?? []
        do { if let image = try await screenshot() { evidence.append(image) } }
        catch { status = "Screenshot unavailable: \(error.localizedDescription). Text will still be saved." }
        feedback["evidence"] = evidence
        try queue(["method": "feedback.save", "feedback": feedback])
        editedFeedback = nil; note = ""; status = "Saved locally; controller delivery will retry automatically."
    }
    func toggleVoice() async throws {
        if !voiceActive {
            guard await AVCaptureDevice.requestAccess(for: .audio) else { throw CaptureError.message("Microphone permission is required for a voice note") }
            try FileManager.default.createDirectory(at: sessionDir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
            let url = sessionDir.appendingPathComponent(UUID().uuidString + ".m4a")
            audio = try AVAudioRecorder(url: url, settings: [AVFormatIDKey: kAudioFormatMPEG4AAC, AVSampleRateKey: 48000, AVNumberOfChannelsKey: 1, AVEncoderAudioQualityKey: AVAudioQuality.high.rawValue])
            guard audio!.record() else { throw CaptureError.message("Microphone did not start") }
            voiceURL = url; voiceStart = offset; voiceActive = true; status = "Microphone on for this voice note. Finish to save and transcribe."; return
        }
        audio?.stop(); audio = nil; voiceActive = false
        guard let url = voiceURL else { return }
        var evidence: [[String: Any]] = [["id": UUID().uuidString, "path": url.path, "kind": "voice", "status": "available", "offset_ms": voiceStart]]
        if let shot = try? await screenshot() { evidence.append(shot) }
        var feedback: [String: Any] = ["id": UUID().uuidString, "revision": 0, "text": "Voice note — transcription pending", "evidence": evidence, "context": uiContext]
        if session["id"] != nil { feedback["session_id"] = session["id"] }
        try queue(["method": "feedback.save", "feedback": feedback])
        feedback["revision"] = 1
        status = "Original audio saved locally. Transcribing on this Mac…"
        do { feedback["text"] = try await transcribe(url); note = feedback["text"] as? String ?? "" }
        catch { feedback["transcription_error"] = error.localizedDescription; note = "Voice note — transcription unavailable"; status = "Audio retained. Transcription failed: " + error.localizedDescription }
        try queue(["method": "feedback.save", "feedback": feedback]); feedback["revision"] = 2
        editedFeedback = feedback
        if feedback["transcription_error"] == nil { status = "Transcript saved. Edit it above and save to correct it; the original audio remains." }
    }
    private func transcribe(_ url: URL) async throws -> String {
        let transcriber = SpeechTranscriber(locale: Locale.current, preset: .transcription)
        if let request = try await AssetInventory.assetInstallationRequest(supporting: [transcriber]) { try await request.downloadAndInstall() }
        let analyzer = SpeechAnalyzer(modules: [transcriber])
        let results = Task { () throws -> String in var text = ""; for try await result in transcriber.results { text += String(result.text.characters) }; return text }
        do { let file = try AVAudioFile(forReading: url); _ = try await analyzer.analyzeSequence(from: file); try await analyzer.finalizeAndFinishThroughEndOfInput(); return try await results.value }
        catch { results.cancel(); throw error }
    }
    func playSession() {
        let items = media.filter { $0["status"] as? String == "available" }.compactMap { $0["path"] as? String }.map { AVPlayerItem(url: URL(fileURLWithPath: $0)) }
        let queue = AVQueuePlayer(items: items); player = queue; queue.play()
    }
    private func handle(_ command: [String: Any]) async throws {
        let destination = command["project"] as? String ?? ""
        if (recording || paused) && !destination.isEmpty && destination != project { throw CaptureError.message("Another project is recording; stop that session before switching") }
        if !destination.isEmpty { switchProject(destination) }
        switch command["method"] as? String {
        case "open": NSApp.activate(ignoringOtherApps: true); if windows.isEmpty { try await loadWindows() }
        case "pause": try await stop(pausing: true)
        case "resume": try await start()
        case "stop": try await stop(pausing: false)
        case "voice": NSApp.activate(ignoringOtherApps: true); try await toggleVoice()
        case "snapshot":
            if let shot = try await screenshot(), let target = command["target"] as? String {
                let response = try await Task.detached { try IPC.call(["method": "state"]) }.value
                if let state = response["state"] as? [String: Any], let notes = state["feedback"] as? [String: [String: Any]], var feedback = notes[target] {
                    var evidence = feedback["evidence"] as? [[String: Any]] ?? []; evidence.append(shot); feedback["evidence"] = evidence
                    try queue(["method": "feedback.save", "feedback": feedback])
                }
            }
        case "play":
            if let source = command["source"] as? [String: Any], let path = source["path"] as? String {
                guard FileManager.default.fileExists(atPath: path) else { throw CaptureError.message("Evidence is unavailable or deleted") }
                if source["kind"] as? String == "screenshot" { preview = NSImage(contentsOfFile: path); player = nil }
                else { player = AVPlayer(url: URL(fileURLWithPath: path)); player?.play() }
                NSApp.activate(ignoringOtherApps: true)
            }
        default: throw CaptureError.message("Unknown capture action")
        }
    }
    private func installShortcut() {
        var type = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
        InstallEventHandler(GetApplicationEventTarget(), { _, _, _ in
            DispatchQueue.main.async { NSApp.activate(ignoringOtherApps: true); NSApp.windows.first?.makeKeyAndOrderFront(nil) }; return noErr
        }, 1, &type, nil, nil)
        let result = RegisterEventHotKey(UInt32(kVK_Space), UInt32(cmdKey | shiftKey), EventHotKeyID(signature: 0x41555256, id: 1), GetApplicationEventTarget(), 0, &hotKey)
        if result != noErr { status = "Shortcut unavailable; use the companion or Autarch's Ctrl+N." }
    }
}

private final class RecordingDelegate: NSObject, SCRecordingOutputDelegate, @unchecked Sendable {
    private let lock = NSLock()
    private var complete = false
    private var failure: Error?
    func recordingOutputDidFinishRecording(_ recordingOutput: SCRecordingOutput) { lock.lock(); complete = true; lock.unlock() }
    func recordingOutput(_ recordingOutput: SCRecordingOutput, didFailWithError error: Error) { lock.lock(); failure = error; complete = true; lock.unlock() }
    private func result() -> (Bool, Error?) { lock.lock(); defer { lock.unlock() }; return (complete, failure) }
    func waitForFinish() async throws { for _ in 0..<100 { let (done,error) = result(); if let error { throw error }; if done { return }; try await Task.sleep(for: .milliseconds(100)) }; throw CaptureError.message("Recording finalization interrupted; original file retained") }
}
