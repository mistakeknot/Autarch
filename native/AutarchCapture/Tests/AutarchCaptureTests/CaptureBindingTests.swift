import XCTest
@testable import AutarchCapture

final class CaptureBindingTests: XCTestCase {
    @MainActor func testSuccessfulStateClearsOnlyControllerConnectionWarning() {
        let model = CaptureModel()
        let ready = model.status
        model.controllerUnavailable(CaptureError.message("Controller unavailable; captures remain local"))
        XCTAssertTrue(model.failed)
        model.controllerUnavailable(CaptureError.message("Controller unavailable; captures remain local"))
        model.applyControllerState([:])
        XCTAssertFalse(model.failed)
        XCTAssertEqual(model.status, ready, "Repeated disconnects must retain the status preceding the connection warning")
    }

    @MainActor func testReconnectPreservesExistingAndInterveningOperationErrors() {
        for message in ["Capture failed", "Local save failed", "Authentication failed"] {
            for errorBeforeDisconnect in [true, false] {
                let model = CaptureModel()
                if errorBeforeDisconnect { model.status = message; model.failed = true }
                model.controllerUnavailable(CaptureError.message("Controller unavailable; captures remain local"))
                if !errorBeforeDisconnect { model.status = message; model.failed = true }
                model.applyControllerState([:])
                XCTAssertTrue(model.failed)
                XCTAssertEqual(model.status, message)
            }
        }
    }

    @MainActor func testSuccessfulStateCannotDismissOutboxRecoveryError() {
        let model = CaptureModel()
        let message = "Capture retained for recovery (concurrent text change); see synthetic outbox"
        model.controllerUnavailable(CaptureError.message(message))
        model.applyControllerState([:])
        XCTAssertTrue(model.failed)
        XCTAssertEqual(model.status, message, "The polling catch also receives durable delivery errors")
    }

    @MainActor func testReconnectCannotClearAnotherOperationWithIdenticalErrorText() {
        let model = CaptureModel()
        let message = "Controller unavailable; captures remain local"
        model.controllerUnavailable(CaptureError.message(message))
        model.status = message; model.failed = true
        model.applyControllerState([:])
        XCTAssertTrue(model.failed, "Another operation owns this status even when its text matches the connection warning")
        XCTAssertEqual(model.status, message)
    }

    @MainActor func testReconnectPreservesRecordingAndOriginalDraftEvidence() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let outbox = try Outbox(directory: directory)
        let model = CaptureModel(outbox: outbox)
        model.applyUIContext(["project": "/first-project", "item": "original-item"])
        await model.quickMoment(activate: {}, capture: { ["id": "original-shot", "status": "available", "kind": "screenshot"] })
        model.note = "Unsaved original draft"
        let ready = model.status
        model.recording = true; model.paused = false; model.voiceActive = true
        let sessionDirectory = model.sessionDir
        model.controllerUnavailable(CaptureError.message("Controller unavailable; captures remain local"))
        model.applyControllerState(["sessions": ["retained": ["id": "retained", "project": "/first-project", "status": "stopped"]], "context": ["project": "/second-project"]])
        XCTAssertFalse(model.failed)
        XCTAssertEqual(model.status, ready)
        XCTAssertTrue(model.recording); XCTAssertFalse(model.paused); XCTAssertTrue(model.voiceActive)
        XCTAssertEqual(model.project, "/first-project")
        XCTAssertEqual(model.sessionDir, sessionDirectory)
        XCTAssertEqual(model.note, "Unsaved original draft")
        XCTAssertEqual(model.retainedSessions.map(\.id), ["retained"], "The successful response must still refresh retained sessions")
        XCTAssertTrue(try outbox.pending().isEmpty, "Reconnection must not submit the draft")
        try await model.saveNote()
        let request = try XCTUnwrap(outbox.pending().first)
        let feedback = try XCTUnwrap(request["feedback"] as? [String: Any])
        XCTAssertEqual(request["project"] as? String, "/first-project")
        XCTAssertEqual(feedback["text"] as? String, "Unsaved original draft")
        XCTAssertEqual((feedback["context"] as? [String: Any])?["item"] as? String, "original-item")
        XCTAssertEqual((feedback["evidence"] as? [[String: Any]])?.first?["id"] as? String, "original-shot")
    }

    @MainActor func testVoiceCorrectionAfterProjectSwitchKeepsOriginalIdentityAndAudio() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let outbox = try Outbox(directory: directory)
        let model = CaptureModel(outbox: outbox)
        model.applyUIContext(["project": "/first-project"])
        model.note = "Original transcript"
        model.retainCorrectionBinding(["id": "voice-note", "project": "/first-project", "revision": 2, "text": "Original transcript", "session_id": "voice-session", "evidence": [["id": "original-audio", "kind": "voice", "status": "available"]]])
        model.applyUIContext(["project": "/second-project"])
        model.note = "Human correction"
        try await model.saveNote()
        let request = try XCTUnwrap(outbox.pending().first)
        let feedback = try XCTUnwrap(request["feedback"] as? [String: Any])
        XCTAssertEqual(request["project"] as? String, "/first-project")
        XCTAssertEqual(request["_base_feedback_text"] as? String, "Original transcript")
        XCTAssertEqual(feedback["id"] as? String, "voice-note")
        XCTAssertEqual(feedback["revision"] as? Int, 2)
        XCTAssertEqual(feedback["session_id"] as? String, "voice-session")
        XCTAssertEqual((feedback["evidence"] as? [[String: Any]])?.first?["id"] as? String, "original-audio")
    }
    @MainActor func testScreenshotOnlyDraftCanBeSavedAfterProjectSwitch() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let outbox = try Outbox(directory: directory)
        let model = CaptureModel(outbox: outbox)
        model.applyUIContext(["project": "/first-project", "item": "first"])
        await model.quickMoment(activate: {}, capture: { ["id": "first-shot", "status": "available", "kind": "screenshot"] })
        model.applyUIContext(["project": "/second-project", "item": "second"])
        var capturedInWrongProject = false
        await model.quickMoment(activate: {}, capture: { capturedInWrongProject = true; return ["id": "wrong-shot", "status": "available"] })
        XCTAssertFalse(capturedInWrongProject, "A later quick moment must not append a new project's screenshot to the old draft")
        try await model.saveNote()
        var pending = try outbox.pending()
        XCTAssertEqual(pending.count, 1, "An image-only moment must not strand the old project draft")
        XCTAssertEqual(pending.first?["project"] as? String, "/first-project")
        XCTAssertEqual(((pending.first?["feedback"] as? [String: Any])?["context"] as? [String: Any])?["item"] as? String, "first")
        await model.quickMoment(activate: {}, capture: { ["id": "second-shot", "status": "available", "kind": "screenshot"] })
        try await model.saveNote()
        pending = try outbox.pending()
        XCTAssertEqual(pending.count, 2)
        XCTAssertEqual(pending.last?["project"] as? String, "/second-project", "Saving must release the previous draft binding")
    }
    @MainActor func testDeletedCachedSessionCannotOwnFutureCaptureFiles() throws {
        for status in ["deleting", "deleted"] {
            let model = CaptureModel(session: ["id": "old-session", "status": "stopped"])
            model.project = "/project"
            model.applySavedSessions(["old-session": ["id": "old-session", "project": "/project", "status": status]])
            XCTAssertEqual(model.sessionDir.lastPathComponent, "intake", "New captures would otherwise be reaped by retention")
            XCTAssertTrue(model.retainedSessions.isEmpty)
        }
    }

    @MainActor func testHotkeyCannotMutateDraftDuringSaveOperation() async throws {
        let model = CaptureModel()
        let started = expectation(description: "save suspended")
        let release = AsyncStream<Void>.makeStream()
        model.perform { started.fulfill(); for await _ in release.stream { break } }
        await fulfillment(of: [started], timeout: 1)
        let status = model.status
        var activated = false
        await model.quickMoment(activate: { activated = true })
        XCTAssertEqual(model.status, status)
        XCTAssertFalse(activated, "Hotkey must not capture or activate while a save owns the draft")
        release.continuation.finish()
    }
    @MainActor func testContextCannotMoveAnInFlightCaptureOperationToAnotherProject() async throws {
        let model = CaptureModel()
        model.applyUIContext(["project": "/first-project"])
        let started = expectation(description: "capture operation suspended")
        let release = AsyncStream<Void>.makeStream()
        model.perform {
            started.fulfill()
            for await _ in release.stream { break }
        }
        await fulfillment(of: [started], timeout: 1)
        model.applyUIContext(["project": "/second-project"])
        XCTAssertEqual(model.project, "/first-project", "Transcription must keep the original note's project while awaiting results")
        release.continuation.finish()
        for _ in 0..<100 {
            await Task.yield()
            model.applyUIContext(["project": "/second-project"])
            if model.project == "/second-project" { return }
        }
        XCTFail("Context did not resume following the workbench after the operation ended")
    }

    @MainActor func testFailedCaptureOperationReleasesItsProjectBinding() async throws {
        let model = CaptureModel()
        model.applyUIContext(["project": "/first-project"])
        let started = expectation(description: "capture operation suspended")
        let release = AsyncStream<Void>.makeStream()
        model.perform {
            started.fulfill()
            for await _ in release.stream { break }
            throw CaptureError.message("Synthetic transcription failure")
        }
        await fulfillment(of: [started], timeout: 1)
        model.applyUIContext(["project": "/second-project"])
        XCTAssertEqual(model.project, "/first-project")
        release.continuation.finish()
        for _ in 0..<100 {
            await Task.yield()
            model.applyUIContext(["project": "/second-project"])
            if model.project == "/second-project" && model.failed { return }
        }
        XCTFail("Failed operation stranded the companion in its previous project")
    }

    @MainActor func testControllerDisconnectCannotReleaseAnInFlightCaptureOperation() async throws {
        let model = CaptureModel()
        model.applyUIContext(["project": "/first-project"])
        let started = expectation(description: "capture operation suspended")
        let release = AsyncStream<Void>.makeStream()
        model.perform {
            started.fulfill()
            for await _ in release.stream { break }
        }
        await fulfillment(of: [started], timeout: 1)
        model.controllerUnavailable(CaptureError.message("Controller unavailable; captures remain local"))
        model.applyControllerState(["context": ["project": "/second-project"]])
        XCTAssertFalse(model.failed)
        XCTAssertEqual(model.project, "/first-project", "An IPC failure must not unlock another operation's project binding")
        release.continuation.finish()
    }
}
