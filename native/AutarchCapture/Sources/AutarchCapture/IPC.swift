import Foundation
import Darwin

enum CaptureError: LocalizedError {
    case message(String)
    case rejected(String)
    var errorDescription: String? { switch self { case .message(let text), .rejected(let text): return text } }
}

final class Outbox {
    let directory: URL
    init(directory: URL) throws {
        self.directory = directory
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
    }
    @discardableResult func enqueue(_ body: [String: Any]) throws -> String {
        var request = body
        let id = request["id"] as? String ?? UUID().uuidString
        request["version"] = 1; request["id"] = id
        let data = try JSONSerialization.data(withJSONObject: request, options: [.sortedKeys])
        let path = directory.appendingPathComponent(id + ".json")
        try data.write(to: path, options: .atomic)
        try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: path.path)
        let handle = try FileHandle(forWritingTo: path); try handle.synchronize(); try handle.close()
        let fd = Darwin.open(directory.path, O_RDONLY); guard fd >= 0 else { throw CaptureError.message("Cannot sync local outbox") }; defer { Darwin.close(fd) }
        guard fsync(fd) == 0 else { throw CaptureError.message("Cannot sync local outbox") }
        return id
    }
    func pending() throws -> [[String: Any]] {
        try FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: [.creationDateKey])
            .filter { $0.pathExtension == "json" && !$0.lastPathComponent.hasSuffix(".rejected.json") }
            .sorted { (try? $0.resourceValues(forKeys: [.creationDateKey]).creationDate) ?? .distantPast < (try? $1.resourceValues(forKeys: [.creationDateKey]).creationDate) ?? .distantPast }
            .map { try JSONSerialization.jsonObject(with: Data(contentsOf: $0)) as! [String: Any] }
    }
    func acknowledge(_ id: String) throws {
        guard !id.contains("/"), !id.contains("..") else { throw CaptureError.message("Invalid receipt") }
        try FileManager.default.removeItem(at: directory.appendingPathComponent(id + ".json"))
    }
    func retainRejected(_ id: String) throws {
        guard !id.contains("/"), !id.contains("..") else { throw CaptureError.message("Invalid receipt") }
        try FileManager.default.moveItem(at: directory.appendingPathComponent(id + ".json"), to: directory.appendingPathComponent(id + ".rejected.json"))
    }
}

// One LF-framed versioned request per private Unix socket connection. Device
// capture is independent of connection lifetime and retries the durable outbox.
enum IPC {
    static let directory = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".autarch/reviews")
    static func call(_ body: [String: Any]) throws -> [String: Any] {
        var request = body; request["version"] = 1
        if request["id"] == nil { request["id"] = UUID().uuidString }
        var data = try JSONSerialization.data(withJSONObject: request); data.append(10)
        let fd = socket(AF_UNIX, SOCK_STREAM, 0); guard fd >= 0 else { throw CaptureError.message("Socket unavailable") }; defer { Darwin.close(fd) }
        var timeout = timeval(tv_sec: 3, tv_usec: 0)
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, socklen_t(MemoryLayout<timeval>.size))
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, socklen_t(MemoryLayout<timeval>.size))
        var noSignal: Int32 = 1; setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &noSignal, socklen_t(MemoryLayout<Int32>.size))
        var address = sockaddr_un(); address.sun_family = sa_family_t(AF_UNIX)
        let path = directory.appendingPathComponent("controller.sock").path
        guard path.utf8.count < MemoryLayout.size(ofValue: address.sun_path) else { throw CaptureError.message("Socket path too long") }
        withUnsafeMutableBytes(of: &address.sun_path) { bytes in _ = path.withCString { strcpy(bytes.baseAddress!.assumingMemoryBound(to: CChar.self), $0) } }
        let connected = withUnsafePointer(to: &address) { pointer in pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { Darwin.connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size)) } }
        guard connected == 0 else { throw CaptureError.message("Controller unavailable; captures remain local") }
        try data.withUnsafeBytes { bytes in
            var offset = 0
            while offset < bytes.count { let n = Darwin.write(fd, bytes.baseAddress!.advanced(by: offset), bytes.count - offset); guard n > 0 else { throw CaptureError.message("Controller write failed") }; offset += n }
        }
        var response = Data(); var buffer = [UInt8](repeating: 0, count: 8192)
        while response.count < 8 * 1024 * 1024 {
            let count = Darwin.read(fd, &buffer, buffer.count)
            guard count > 0 else { throw CaptureError.message("Controller disconnected before acknowledgement") }
            response.append(contentsOf: buffer.prefix(count))
            if response.contains(10) { break }
        }
        guard let result = try JSONSerialization.jsonObject(with: response) as? [String: Any], result["version"] as? Int == 1 else { throw CaptureError.message("Controller version mismatch") }
        if let error = result["error"] as? String, !error.isEmpty {
            if error.hasPrefix("not saved:") { throw CaptureError.message(error) }
            throw CaptureError.rejected(error)
        }
        return result
    }
}
