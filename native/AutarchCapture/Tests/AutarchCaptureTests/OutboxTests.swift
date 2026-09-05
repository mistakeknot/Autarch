import XCTest
@testable import AutarchCapture

final class OutboxTests: XCTestCase {
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
