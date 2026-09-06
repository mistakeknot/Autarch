import AppKit
import XCTest
@testable import AutarchCapture

final class ProviderConnectionPanelTests: XCTestCase {
    @MainActor func testSignInWindowIsSeparateAndClearsSensitiveInstructions() {
        _ = NSApplication.shared
        let panel = ProviderConnectionPanel()
        panel.update(["runtimeId": "fixture", "operation": ["id": "operation", "status": "pending", "events": [["type": "device_code", "userCode": "SYNTHETIC-CODE", "verificationUri": "https://example.test/device"]], "prompt": ["id": "prompt", "type": "secret", "message": "Synthetic credential"]]])
        guard let window = NSApp.windows.first(where: { $0.title == "Autarch · Connect provider" }), let content = window.contentView else { return XCTFail("Separate sign-in window missing") }
        func descendants(_ view: NSView) -> [NSView] { [view] + view.subviews.flatMap(descendants) }
        let views = descendants(content)
        XCTAssertNotNil(views.first { $0 is NSSecureTextField }, "Credential entry must use a native secure field")
        let text = views.compactMap { $0 as? NSTextView }.first { $0.string.contains("SYNTHETIC-CODE") }
        XCTAssertNotNil(text, "Device instructions must appear in the excluded companion window")
        window.close()
        panel.update(["runtimeId": "fixture", "displayId": "reopened", "operation": ["id": "operation", "status": "pending", "events": [["type": "device_code", "userCode": "SYNTHETIC-CODE"]]]])
        XCTAssertTrue(window.isVisible, "Reopening Connect provider must recover the same outstanding operation")
        panel.update(["runtimeId": "fixture", "operation": ["id": "operation", "status": "connected"]])
        XCTAssertEqual(text?.string, "")
        XCTAssertFalse(window.isVisible)
    }
}
