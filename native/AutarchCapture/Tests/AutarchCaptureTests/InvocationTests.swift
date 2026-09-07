import XCTest
@testable import AutarchCapture

final class InvocationTests: XCTestCase {
    func testPaneVerificationRequiresAllOriginalClientsStillShowingThatPane() {
        XCTAssertTrue(invokingPaneMatches("100\t%7\n200\t%8\n", pane: "%7", clientPIDs: [100]))
        XCTAssertFalse(invokingPaneMatches("100\t%8\n", pane: "%7", clientPIDs: [100]))
        XCTAssertFalse(invokingPaneMatches("200\t%7\n", pane: "%7", clientPIDs: [100]))
        XCTAssertFalse(invokingPaneMatches("100\t%7\n200\t%8\n", pane: "%7", clientPIDs: [100, 200]))
        XCTAssertFalse(invokingPaneMatches("100\t%7\n", pane: "%7", clientPIDs: []))
    }
    @MainActor func testSameWindowWithUnverifiedPaneCannotReuseDraftContext() {
        let model = CaptureModel()
        model.applyInvocationContext(["project": "/old", "terminal": ["pid": 123, "window_id": UInt32(11), "pane": "%7"]])
        model.note = "Original pane draft"
        XCTAssertFalse(model.prepareShortcutInvocation(InvocationWindow(id: 11, pid: 42)))
    }

    @MainActor func testNewWindowDetachesStoppedSessionButPreservesExistingDraftBinding() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        let box = try Outbox(directory: dir)
        let model = CaptureModel(session: ["id": "old-session", "status": "stopped"], outbox: box)
        model.project = "/same"
        model.note = "Old session draft"
        model.applyInvocationContext(["project": "/same", "terminal": ["pid": 123, "window_id": UInt32(22)]])
        XCTAssertEqual(model.sessionDir.lastPathComponent, "intake")
        try await model.saveNote()
        let feedback = try XCTUnwrap(try box.pending().first?["feedback"] as? [String: Any])
        XCTAssertEqual(feedback["session_id"] as? String, "old-session")
        model.note = "New window feedback"
        try await model.saveNote()
        let second = try XCTUnwrap(try box.pending().last?["feedback"] as? [String: Any])
        XCTAssertNil(second["session_id"])
    }
    @MainActor func testShortcutFromDifferentTerminalDoesNotReusePreviousProject() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        let box = try Outbox(directory: dir)
        let model = CaptureModel(outbox: box)
        model.applyInvocationContext(["project": "/old", "terminal": ["pid": 123, "window_id": UInt32(11)]])
        XCTAssertTrue(model.prepareShortcutInvocation(InvocationWindow(id: 22, pid: 42)))
        model.note = "New terminal observation"
        try await model.saveNote()
        let request = try XCTUnwrap(box.pending().first)
        XCTAssertEqual(request["project"] as? String, "")
        let feedback = try XCTUnwrap(request["feedback"] as? [String: Any])
        let context = try XCTUnwrap(feedback["context"] as? [String: Any])
        XCTAssertEqual((context["terminal"] as? [String: Any])?["window_id"] as? UInt32, 22)
    }

    @MainActor func testShortcutCannotAppendAnotherTerminalToAnExistingDraft() {
        let model = CaptureModel()
        model.applyInvocationContext(["project": "/old", "terminal": ["pid": 123, "window_id": UInt32(11)]])
        model.note = "Unsaved draft"
        XCTAssertFalse(model.prepareShortcutInvocation(InvocationWindow(id: 22, pid: 42)))
        XCTAssertEqual(model.project, "/old")
        XCTAssertEqual(model.note, "Unsaved draft")
    }
    @MainActor func testInvocationKeepsFeedbackBoundToOriginAfterAnotherTUIUpdatesContext() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: dir) }
        let box = try Outbox(directory: dir)
        let model = CaptureModel(outbox: box)
        model.applyInvocationContext(["project": "/origin", "item": "source", "terminal": ["pid": 123, "pane": "%7"]])
        model.applyUIContext(["project": "/other", "terminal": ["pid": 456]])
        model.note = "Human observation"
        try await model.saveNote()
        let request = try XCTUnwrap(box.pending().first)
        XCTAssertEqual(request["project"] as? String, "/origin")
        let feedback = try XCTUnwrap(request["feedback"] as? [String: Any])
        let context = try XCTUnwrap(feedback["context"] as? [String: Any])
        XCTAssertEqual(context["item"] as? String, "source")
        XCTAssertEqual((context["terminal"] as? [String: Any])?["pane"] as? String, "%7")
    }

    func testWindowSelectionRequiresForegroundOriginAndUsesWindowOrder() {
        let windows = [InvocationWindow(id: 90, pid: 99), InvocationWindow(id: 11, pid: 42), InvocationWindow(id: 12, pid: 42)]
        XCTAssertEqual(selectInvocationWindow(windows, processIDs: [42], frontmostPID: 42), 11)
        XCTAssertNil(selectInvocationWindow(windows, processIDs: [42], frontmostPID: 99))
        XCTAssertNil(selectInvocationWindow(windows, processIDs: [], frontmostPID: 42))
    }

    func testRecordedWindowDoesNotFollowFocusToAnotherWindowOfSameTerminal() {
        let windows = [InvocationWindow(id: 12, pid: 42), InvocationWindow(id: 11, pid: 42)]
        XCTAssertEqual(selectRecordedInvocationWindow(windows, windowID: 11, processIDs: [42]), 11)
        XCTAssertNil(selectRecordedInvocationWindow(windows, windowID: 13, processIDs: [42]))
        XCTAssertNil(selectRecordedInvocationWindow(windows, windowID: 11, processIDs: [99]))
    }
}
