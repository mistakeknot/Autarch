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
    @Published var retainedSessions: [RetainedSession] = []
    @Published var selectedRetainedSession = ""
    @Published var capturePermissionNeeded = false
    private var draftEvidence: [[String: Any]] = []
    private var draftProject: String?
    private var draftContext: [String: Any]?
    private var draftSessionID: String?
    private(set) var media: [[String: Any]] = []
    private var session: [String: Any] = [:]
    private var stream: SCStream?
    private var output: SCRecordingOutput?
    private var recorderDelegate: RecordingDelegate?
    private var audio: AVAudioRecorder?
    private var voiceURL: URL?
    private var voiceStart: Int64 = 0
    private var voiceSessionID: String?
    private var voiceProject = ""
    private var voiceContext: [String: Any] = [:]
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
    var sessionDir: URL { IPC.directory.appendingPathComponent("media/" + (session["id"] as? String ?? "intake")) }
    private var offset: Int64 { Int64(Date().timeIntervalSince(sessionStart) * 1000) }

    init(session: [String: Any] = [:], outbox: Outbox? = nil) { self.session = session; self.outbox = outbox; super.init() }

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
                    try box.deliverPending(call: IPC.call)
                    return try IPC.call(["method": "state"])
                }.value
                if let state = response["state"] as? [String: Any] {
                    if let saved = state["sessions"] as? [String: [String: Any]] {
                        applySavedSessions(saved)
                    }
                    if let context = state["context"] as? [String: Any] { applyUIContext(context) }
                    if !recovered { try await recoverInterruptedSessions(); recovered = true }
                    if !working, let commands = state["commands"] as? [[String: Any]], let command = commands.first(where: { $0["status"] as? String == "pending" }) {
                        working = true
                        defer { working = false }
                        do { try await handle(command); try queue(["method": "capture.ack", "target": command["id"]!, "status": "done"]) }
                        catch { status = error.localizedDescription; failed = true; try queue(["method": "capture.ack", "target": command["id"]!, "status": "failed: " + error.localizedDescription]) }
                    }
                }
                let size = response["storage_bytes"] as? Int64 ?? 0
                usage = ByteCountFormatter.string(fromByteCount: size, countStyle: .file) + " retained locally"
                if recording && Date().timeIntervalSince(lastHeartbeat) > 5 { try persistSession("recording"); lastHeartbeat = Date() }
            } catch { controllerUnavailable(error) }
            try? await Task.sleep(for: .seconds(1))
        }
    }
    func applySavedSessions(_ saved: [String: [String: Any]]) {
        if let id = session["id"] as? String, let current = saved[id], ["deleting", "deleted"].contains(current["status"] as? String ?? "") {
            session = [:]; media = []; player = nil; preview = nil
        }
        retainedSessions = saved.values.filter { $0["project"] as? String == project && !["deleting", "deleted"].contains($0["status"] as? String ?? "") }.map { RetainedSession(id: $0["id"] as? String ?? "", title: ($0["window_title"] as? String ?? "Session") + " · " + ($0["started_at"] as? String ?? ""), media: $0["media"] as? [[String: Any]] ?? []) }.sorted { $0.title > $1.title }
        if !retainedSessions.contains(where: { $0.id == selectedRetainedSession }) { selectedRetainedSession = "" }
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
    func applyUIContext(_ context: [String: Any]) {
        let destination = context["project"] as? String ?? project
        // Async capture/transcription retains its original project and context.
        // Reconnecting to a different TUI must not redirect its pending writes.
        guard destination == project || (!recording && !paused && !voiceActive && !working) else { return }
        switchProject(destination)
        uiContext = context
    }
    func controllerUnavailable(_ error: Error) {
        // The operation that acquired working owns its release, even while IPC
        // is unavailable. Its original evidence remains in the local outbox.
        status = error.localizedDescription; failed = true
    }
    private func switchProject(_ destination: String) {
        guard project != destination else { return }
        if !note.isEmpty && draftProject == nil { draftProject = project; draftContext = uiContext; draftSessionID = session["id"] as? String }
        project = destination; session = [:]; media = []
        if draftProject == nil { editedFeedback = nil }
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
    func openCaptureSettings() {
        NSWorkspace.shared.open(URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture")!)
    }
    func loadWindows() async throws {
        if !CGPreflightScreenCaptureAccess() {
            capturePermissionNeeded = true
            guard CGRequestScreenCaptureAccess() else { throw CaptureError.message("Allow AutarchCapture in Screen & System Audio Recording, then refresh windows. Typed notes remain available.") }
        }
        capturePermissionNeeded = false
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
            if draftProject == nil { editedFeedback = nil }
            sessionStart = Date(); media = []
            session = ["id": UUID().uuidString, "revision": 0, "window_id": selectedWindow, "window_title": "\(window.owningApplication?.applicationName ?? "App") — \(window.title ?? "Untitled")"]
        }
        try FileManager.default.createDirectory(at: sessionDir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let url = sessionDir.appendingPathComponent(UUID().uuidString + ".mp4")
        let configuration = SCRecordingOutputConfiguration(); configuration.outputURL = url
        let delegate = RecordingDelegate { [weak self] error in Task { @MainActor in self?.captureInterrupted(error) } }; recorderDelegate = delegate
        let output = SCRecordingOutput(configuration: configuration, delegate: delegate); self.output = output
        let stream = SCStream(filter: SCContentFilter(desktopIndependentWindow: window), configuration: self.configuration(window), delegate: delegate); self.stream = stream
        try stream.addRecordingOutput(output)
        segmentStart = Date()
        media.append(["id": UUID().uuidString, "path": url.path, "status": "recording", "kind": "video", "offset_ms": offset])
        try persistSession("starting")
        do { try await stream.startCapture(); recording = true; paused = false; try persistSession("recording"); status = "Recording only the selected window. Microphone is off." }
        catch { session["error"] = error.localizedDescription; try? persistSession("interrupted"); throw error }
    }
    private func captureInterrupted(_ error: Error) {
        guard recording || session["status"] as? String == "starting" else { return }
        recording = false; paused = false
        if !media.isEmpty { media[media.count - 1]["status"] = "unavailable" }
        session["error"] = error.localizedDescription
        do { try persistSession("interrupted") }
        catch { status = "Recording interrupted; original files retained. Session update is pending: " + error.localizedDescription; failed = true; return }
        status = "Recording interrupted; original files retained: " + error.localizedDescription; failed = true
    }
    func stop(pausing: Bool) async throws {
        guard recording || paused else { throw CaptureError.message("No active review recording") }
        if voiceActive { try await toggleVoice() }
        if let stream, recording {
            do {
                try await stream.stopCapture()
                if let delegate = recorderDelegate { try await delegate.waitForFinish() }
            } catch {
                recording = false; paused = false; self.stream = nil; output = nil
                if !media.isEmpty { media[media.count - 1]["status"] = "unavailable" }
                session["error"] = error.localizedDescription
                try persistSession("interrupted")
                throw error
            }
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
        guard !note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !draftEvidence.isEmpty else { return }
        var feedback = editedFeedback ?? ["id": UUID().uuidString, "revision": 0, "context": draftContext ?? uiContext]
        let baseText = editedFeedback?["text"] as? String
        feedback["text"] = note
        if let id = draftSessionID ?? (draftProject == nil ? session["id"] as? String : nil) { feedback["session_id"] = id }
        var evidence = feedback["evidence"] as? [[String: Any]] ?? []
        evidence.append(contentsOf: draftEvidence)
        do { if draftEvidence.isEmpty && draftProject == nil, let image = try await screenshot() { evidence.append(image) } }
        catch { status = "Screenshot unavailable: \(error.localizedDescription). Text will still be saved." }
        feedback["evidence"] = evidence
        var request: [String: Any] = ["method": "feedback.save", "project": feedback["project"] as? String ?? draftProject ?? project, "feedback": feedback]
        request["_base_feedback_text"] = baseText
        try queue(request)
        editedFeedback = nil; note = ""; draftEvidence = []; draftProject = nil; draftContext = nil; draftSessionID = nil
        status = "Saved locally; controller delivery will retry automatically."
    }
    func toggleVoice() async throws {
        if !voiceActive {
            if let draftProject, draftProject != project { throw CaptureError.message("Save your existing note before recording in the newly selected project") }
            if !note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !draftEvidence.isEmpty { try await saveNote() }
            guard await AVCaptureDevice.requestAccess(for: .audio) else { throw CaptureError.message("Microphone permission is required for a voice note") }
            try FileManager.default.createDirectory(at: sessionDir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
            let url = sessionDir.appendingPathComponent(UUID().uuidString + ".m4a")
            audio = try AVAudioRecorder(url: url, settings: [AVFormatIDKey: kAudioFormatMPEG4AAC, AVSampleRateKey: 48000, AVNumberOfChannelsKey: 1, AVEncoderAudioQualityKey: AVAudioQuality.high.rawValue])
            guard audio!.record() else { throw CaptureError.message("Microphone did not start") }
            voiceSessionID = session["id"] as? String; voiceProject = project; voiceContext = uiContext
            voiceURL = url; voiceStart = offset; voiceActive = true; status = "Microphone on for this voice note. Finish to save and transcribe."; return
        }
        audio?.stop(); audio = nil; voiceActive = false
        guard let url = voiceURL else { return }
        var evidence: [[String: Any]] = [["id": UUID().uuidString, "path": url.path, "kind": "voice", "status": "available", "offset_ms": voiceStart]]
        if voiceSessionID == session["id"] as? String, let shot = try? await screenshot() { evidence.append(shot) }
        var feedback: [String: Any] = ["id": UUID().uuidString, "revision": 0, "project": voiceProject, "text": "Voice note — transcription pending", "evidence": evidence, "context": voiceContext]
        feedback["session_id"] = voiceSessionID
        try queue(["method": "feedback.save", "project": voiceProject, "feedback": feedback])
        feedback["revision"] = 1
        let baseText = feedback["text"] as! String
        status = "Original audio saved locally. Transcribing on this Mac…"
        do { feedback["text"] = try await transcribe(url); note = feedback["text"] as? String ?? "" }
        catch { feedback["transcription_error"] = error.localizedDescription; note = "Voice note — transcription unavailable"; status = "Audio retained. Transcription failed: " + error.localizedDescription }
        try queue(["method": "feedback.save", "project": voiceProject, "feedback": feedback, "_base_feedback_text": baseText]); feedback["revision"] = 2
        retainCorrectionBinding(feedback)
        if feedback["transcription_error"] == nil { status = "Transcript saved. Edit it above and save to correct it; the original audio remains." }
    }
    func retainCorrectionBinding(_ feedback: [String: Any]) {
        editedFeedback = feedback
        draftProject = feedback["project"] as? String ?? project
        draftContext = feedback["context"] as? [String: Any] ?? uiContext
        draftSessionID = feedback["session_id"] as? String
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
    func playRetainedSession() {
        guard let selected = retainedSessions.first(where: { $0.id == selectedRetainedSession }) else { return }
        let items = selected.media.filter { $0["status"] as? String == "available" }.compactMap { $0["path"] as? String }.map { AVPlayerItem(url: URL(fileURLWithPath: $0)) }
        let queue = AVQueuePlayer(items: items); preview = nil; player = queue; queue.play()
    }
    func quickMoment(activate: (() -> Void)? = nil, capture: (() async throws -> [String: Any]?)? = nil) async {
        guard !working else { return }; working = true; defer { working = false }
        if let draftProject, draftProject != project {
            status = "Save your existing review moment before capturing in the newly selected project."
            return
        }
        if draftProject == nil { draftProject = project; draftContext = uiContext; draftSessionID = session["id"] as? String }
        do {
            let shot: [String: Any]?
            if let capture { shot = try await capture() } else { shot = try await screenshot() }
            if let shot { draftEvidence.append(shot) }
            status = "Review moment captured. Type or speak, then save."
        }
        catch { status = "Screenshot unavailable. Your note can still be saved: " + error.localizedDescription }
        if let activate { activate() }
        else { NSApp.activate(ignoringOtherApps: true); NSApp.windows.first?.makeKeyAndOrderFront(nil) }
    }
    private func handle(_ command: [String: Any]) async throws {
        let destination = command["project"] as? String ?? ""
        if (recording || paused || voiceActive) && !destination.isEmpty && destination != project { throw CaptureError.message("Another project is recording; stop that session before switching") }
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
                    try queue(["method": "feedback.save", "project": feedback["project"] as? String ?? destination, "feedback": feedback, "_base_feedback_text": feedback["text"] as? String ?? ""])
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
        InstallEventHandler(GetApplicationEventTarget(), { _, _, pointer in
            guard let pointer else { return noErr }
            let model = Unmanaged<CaptureModel>.fromOpaque(pointer).takeUnretainedValue()
            Task { @MainActor in await model.quickMoment() }; return noErr
        }, 1, &type, Unmanaged.passUnretained(self).toOpaque(), nil)
        let result = RegisterEventHotKey(UInt32(kVK_Space), UInt32(cmdKey | shiftKey), EventHotKeyID(signature: 0x41555256, id: 1), GetApplicationEventTarget(), 0, &hotKey)
        if result != noErr { status = "Shortcut unavailable; use the companion or Autarch's Ctrl+N." }
    }
}

struct RetainedSession: Identifiable {
    let id: String
    let title: String
    let media: [[String: Any]]
}

private final class RecordingDelegate: NSObject, SCRecordingOutputDelegate, SCStreamDelegate, @unchecked Sendable {
    private let onFailure: @Sendable (Error) -> Void
    init(onFailure: @escaping @Sendable (Error) -> Void) { self.onFailure = onFailure }
    func stream(_ stream: SCStream, didStopWithError error: Error) { onFailure(error) }
    private let lock = NSLock()
    private var complete = false
    private var failure: Error?
    func recordingOutputDidFinishRecording(_ recordingOutput: SCRecordingOutput) { lock.lock(); complete = true; lock.unlock() }
    func recordingOutput(_ recordingOutput: SCRecordingOutput, didFailWithError error: Error) { lock.lock(); failure = error; complete = true; lock.unlock(); onFailure(error) }
    private func result() -> (Bool, Error?) { lock.lock(); defer { lock.unlock() }; return (complete, failure) }
    func waitForFinish() async throws { for _ in 0..<100 { let (done,error) = result(); if let error { throw error }; if done { return }; try await Task.sleep(for: .milliseconds(100)) }; throw CaptureError.message("Recording finalization interrupted; original file retained") }
}
