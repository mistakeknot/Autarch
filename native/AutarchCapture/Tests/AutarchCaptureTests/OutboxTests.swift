import XCTest
@testable import AutarchCapture

final class OutboxTests: XCTestCase {
    func testCrashAfterRebaseEnqueueDrainsBothCopiesWithoutFalseRecovery() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        var edit: [String: Any] = ["id": "note", "revision": 1, "text": "Correction", "evidence": [["id": "shot", "status": "available"]]]
        try box.enqueue(["method": "feedback.save", "project": "/project", "_base_feedback_text": "Original", "feedback": edit])
        edit["revision"] = 2
        try box.enqueue(["method": "feedback.save", "project": "/project", "_base_feedback_text": "Original", "feedback": edit])
        var current: [String: Any] = ["id": "note", "project": "/project", "revision": 2, "text": "Original", "evidence": []]
        var writes = 0
        try box.deliverPending { request in
            if request["method"] as? String == "state" { return ["state": ["feedback": ["note": current]]] }
            let candidate = request["feedback"] as! [String: Any]
            guard candidate["revision"] as? Int == current["revision"] as? Int else { throw CaptureError.rejected("stale feedback") }
            current = candidate; current["revision"] = (candidate["revision"] as! Int) + 1; writes += 1
            return ["id": "note"]
        }
        XCTAssertEqual(writes, 1)
        XCTAssertTrue(try box.pending().isEmpty)
        XCTAssertTrue(try FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil).isEmpty)
    }

    func testSameTextDoesNotAcknowledgeAnUnappliedScreenshot() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        try box.enqueue(["method": "feedback.save", "project": "/project", "_base_feedback_text": "Same text", "feedback": ["id": "note", "revision": 1, "text": "Same text", "evidence": [["id": "new-shot", "status": "available"]]]])
        var saves = 0
        try box.deliverPending { request in
            if request["method"] as? String == "state" { return ["state": ["feedback": ["note": ["id": "note", "project": "/project", "revision": 2, "text": "Same text", "evidence": []]]]] }
            saves += 1
            if saves == 1 { throw CaptureError.rejected("stale feedback") }
            XCTAssertEqual(((request["feedback"] as? [String: Any])?["evidence"] as? [[String: Any]])?.first?["id"] as? String, "new-shot")
            return ["id": "note"]
        }
        XCTAssertEqual(saves, 2)
    }

    func testSessionBindingRejectionIsRetainedWithoutAutomaticRebase() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        let id = try box.enqueue(["method": "feedback.save", "project": "/project", "_base_feedback_text": "Original", "feedback": ["id": "note", "revision": 1, "text": "Correction", "session_id": "wrong-session"]])
        var calls = 0
        XCTAssertThrowsError(try box.deliverPending { request in
            calls += 1
            XCTAssertEqual(request["method"] as? String, "feedback.save")
            throw CaptureError.rejected("feedback session project mismatch")
        })
        XCTAssertEqual(calls, 1)
        XCTAssertTrue(FileManager.default.fileExists(atPath: directory.appendingPathComponent(id + ".rejected.json").path))
    }
    func testRebasedCorrectionKeepsItsExactRetryAfterLostAcknowledgement() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        try box.enqueue(["method": "feedback.save", "project": "/project", "_base_feedback_text": "Original", "feedback": ["id": "note", "revision": 1, "text": "Correction"]])
        var committed: [String: Any]?
        var saves = 0
        XCTAssertThrowsError(try box.deliverPending { request in
            if request["method"] as? String == "state" { return ["state": ["feedback": ["note": ["id": "note", "project": "/project", "revision": 2, "text": "Original"]]]] }
            saves += 1
            if saves == 1 { throw CaptureError.rejected("stale feedback") }
            committed = request
            throw CaptureError.message("Acknowledgement lost")
        })
        let expected = try JSONSerialization.data(withJSONObject: XCTUnwrap(committed), options: [.sortedKeys])
        let reopened = try Outbox(directory: directory)
        try reopened.deliverPending { request in
            XCTAssertEqual(try JSONSerialization.data(withJSONObject: request, options: [.sortedKeys]), expected)
            return ["id": "note", "replayed": true]
        }
        XCTAssertTrue(try reopened.pending().isEmpty)
    }

    func testRepeatedMetadataChangesLeaveOneDurableBoundedRetry() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        try box.enqueue(["method": "feedback.save", "project": "/project", "_base_feedback_text": "Original", "feedback": ["id": "note", "revision": 1, "text": "Correction"]])
        var revision = 1
        XCTAssertThrowsError(try box.deliverPending { request in
            if request["method"] as? String == "state" {
                revision += 1
                return ["state": ["feedback": ["note": ["id": "note", "project": "/project", "revision": revision, "text": "Original"]]]]
            }
            throw CaptureError.rejected("stale feedback")
        })
        XCTAssertEqual(revision, 4, "A busy store must not spin the delivery loop forever")
        XCTAssertEqual(try box.pending().count, 1)
        try box.deliverPending { request in
            XCTAssertEqual((request["feedback"] as? [String: Any])?["revision"] as? Int, revision)
            return ["id": "note"]
        }
        XCTAssertTrue(try box.pending().isEmpty)
    }
    func testTranscriptRebasesRetentionRevisionAndIntakeRouting() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        let original = try box.enqueue(["method": "feedback.save", "project": "", "_base_feedback_text": "Transcribing", "feedback": ["id": "note", "revision": 1, "text": "Correct transcript", "evidence": [["id": "voice", "status": "available", "path": "/deleted/audio"]]]])
        var saves = 0
        try box.deliverPending { request in
            if request["method"] as? String == "state" {
                return ["state": ["feedback": ["note": ["id": "note", "project": "/routed", "revision": 3, "text": "Transcribing", "evidence": [["id": "voice", "status": "deleted"]]]]]]
            }
            saves += 1
            if saves == 1 { throw CaptureError.rejected("project mismatch") }
            let feedback = try XCTUnwrap(request["feedback"] as? [String: Any])
            XCTAssertEqual(request["project"] as? String, "/routed")
            XCTAssertEqual(feedback["revision"] as? Int, 3)
            XCTAssertEqual(feedback["text"] as? String, "Correct transcript")
            XCTAssertEqual((feedback["evidence"] as? [[String: Any]])?.first?["status"] as? String, "deleted")
            XCTAssertNotEqual(request["id"] as? String, original, "A rebased payload must have its own retry identity")
            return ["id": "note"]
        }
        XCTAssertEqual(saves, 2)
        XCTAssertTrue(try box.pending().isEmpty)
    }

    func testConflictingHumanCorrectionRemainsRecoverableWithoutOverwrite() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        let id = try box.enqueue(["method": "feedback.save", "project": "/project", "_base_feedback_text": "Transcribing", "feedback": ["id": "note", "revision": 1, "text": "Automatic transcript"]])
        var saves = 0
        XCTAssertThrowsError(try box.deliverPending { request in
            if request["method"] as? String == "state" { return ["state": ["feedback": ["note": ["id": "note", "project": "/project", "revision": 2, "text": "Human correction"]]]] }
            saves += 1
            throw CaptureError.rejected("stale feedback")
        })
        XCTAssertEqual(saves, 1)
        let retained = directory.appendingPathComponent(id + ".rejected.json")
        XCTAssertTrue(FileManager.default.fileExists(atPath: retained.path))
        XCTAssertTrue(try String(contentsOf: retained, encoding: .utf8).contains("Automatic transcript"))
    }
    func testRejectedRequestRemainsLocalWithoutBlockingNextCapture() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        let first = try box.enqueue(["method": "feedback.save", "text": "Needs routing repair"])
        try box.enqueue(["method": "feedback.save", "text": "Independent note"])
        try box.retainRejected(first)
        XCTAssertEqual(try box.pending().count, 1)
        XCTAssertTrue(FileManager.default.fileExists(atPath: directory.appendingPathComponent(first + ".rejected.json").path))
    }
    func testUnavailableControllerRetainsOriginalRequestAcrossReopen() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let box = try Outbox(directory: directory)
        let id = try box.enqueue(["method": "feedback.save", "text": "Original observation"])
        let reopened = try Outbox(directory: directory)
        XCTAssertEqual(try reopened.pending().count, 1)
        let request = try XCTUnwrap(reopened.pending().first)
        XCTAssertEqual(request["id"] as? String, id)
        XCTAssertEqual(request["text"] as? String, "Original observation")
        try reopened.acknowledge(id)
        XCTAssertTrue(try box.pending().isEmpty)
    }
}
