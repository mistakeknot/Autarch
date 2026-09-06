import XCTest
import Carbon
import SwiftUI
@testable import AutarchCapture

@MainActor final class CaptureShortcutTests: XCTestCase {
    func testDefaultAvoidsAlfredAndSavedShortcutSurvivesRelaunch() throws {
        let defaults = UserDefaults(suiteName: "autarch-shortcut-test-" + UUID().uuidString)!
        let registrar = TestShortcutRegistrar()
        let controller = CaptureShortcutController(defaults: defaults, registrar: registrar)
        controller.start(action: {})
        XCTAssertEqual(controller.active, .suggested)
        XCTAssertNotEqual(controller.active?.modifiers, UInt32(cmdKey | shiftKey))
        let custom = CaptureShortcut(keyCode: UInt32(kVK_ANSI_R), modifiers: UInt32(controlKey | shiftKey))
        XCTAssertTrue(controller.apply(custom))
        let relaunched = CaptureShortcutController(defaults: defaults, registrar: TestShortcutRegistrar())
        relaunched.start(action: {})
        XCTAssertEqual(relaunched.active, custom)
        XCTAssertTrue(relaunched.apply(nil))
        let disabled = CaptureShortcutController(defaults: defaults, registrar: TestShortcutRegistrar())
        disabled.start(action: {})
        XCTAssertNil(disabled.active)
    }

    func testConflictKeepsWorkingShortcutAndSavedPreference() {
        let defaults = UserDefaults(suiteName: "autarch-shortcut-test-" + UUID().uuidString)!
        let registrar = TestShortcutRegistrar()
        let controller = CaptureShortcutController(defaults: defaults, registrar: registrar)
        controller.start(action: {})
        registrar.reject = true
        XCTAssertFalse(controller.apply(CaptureShortcut(keyCode: UInt32(kVK_ANSI_A), modifiers: UInt32(cmdKey | shiftKey))))
        XCTAssertEqual(controller.active, .suggested)
        XCTAssertTrue(controller.error.contains("already in use"))
        let relaunched = CaptureShortcutController(defaults: defaults, registrar: TestShortcutRegistrar())
        relaunched.start(action: {})
        XCTAssertEqual(relaunched.active, .suggested)
    }

    func testInvalidShortcutCannotConsumeOrdinaryTyping() {
        let registrar = TestShortcutRegistrar()
        let controller = CaptureShortcutController(defaults: UserDefaults(suiteName: UUID().uuidString)!, registrar: registrar)
        controller.start(action: {})
        XCTAssertFalse(controller.apply(CaptureShortcut(keyCode: UInt32(kVK_ANSI_A), modifiers: UInt32(shiftKey))))
        XCTAssertFalse(controller.apply(CaptureShortcut(keyCode: 999, modifiers: UInt32(cmdKey))))
        XCTAssertEqual(controller.active, .suggested)
        XCTAssertEqual(registrar.current, .suggested)
    }

    func testStartupFailureIsVisibleAndDoesNotClaimActiveShortcut() {
        let registrar = TestShortcutRegistrar(); registrar.reject = true
        let controller = CaptureShortcutController(defaults: UserDefaults(suiteName: UUID().uuidString)!, registrar: registrar)
        controller.start(action: {})
        XCTAssertNil(controller.active)
        XCTAssertFalse(controller.error.isEmpty)
    }

    func testRealCarbonConflictAndDisablePreserveOwnership() throws {
        _ = NSApplication.shared
        let owner = CarbonCaptureHotKey(), blocker = CarbonCaptureHotKey(), probe = CarbonCaptureHotKey()
        let first = CaptureShortcut(keyCode: UInt32(kVK_F11), modifiers: CaptureShortcut.modifierMask)
        let second = CaptureShortcut(keyCode: UInt32(kVK_F12), modifiers: CaptureShortcut.modifierMask)
        try owner.replace(with: first)
        try blocker.replace(with: second)
        XCTAssertThrowsError(try owner.replace(with: second))
        XCTAssertThrowsError(try probe.replace(with: first))
        try owner.replace(with: nil)
        try probe.replace(with: first)
    }

    func testSeparateHotKeysDispatchToTheirOwnCallbacks() throws {
        _ = NSApplication.shared
        let first = CarbonCaptureHotKey(), second = CarbonCaptureHotKey()
        var calls: [String] = []
        first.onInvoke = { calls.append("first") }
        second.onInvoke = { calls.append("second") }
        try first.replace(with: CaptureShortcut(keyCode: UInt32(kVK_F9), modifiers: CaptureShortcut.modifierMask))
        try second.replace(with: CaptureShortcut(keyCode: UInt32(kVK_F10), modifiers: CaptureShortcut.modifierMask))
        for owner in [first, second] {
            var event: EventRef?
            XCTAssertEqual(CreateEvent(nil, OSType(kEventClassKeyboard), UInt32(kEventHotKeyPressed), GetCurrentEventTime(), EventAttributes(kEventAttributeUserEvent), &event), noErr)
            let keyEvent = try XCTUnwrap(event)
            var id = EventHotKeyID(signature: 0x41555256, id: owner.registrationID)
            XCTAssertEqual(SetEventParameter(keyEvent, EventParamName(kEventParamDirectObject), EventParamType(typeEventHotKeyID), MemoryLayout<EventHotKeyID>.size, &id), noErr)
            XCTAssertEqual(SendEventToEventTarget(keyEvent, GetApplicationEventTarget()), noErr)
        }
        XCTAssertEqual(calls, ["first", "second"])
    }

    func testSettingsWindowRenders() throws {
        guard let path = ProcessInfo.processInfo.environment["AUTARCH_SHORTCUT_QA_IMAGE"] else { return }
        _ = NSApplication.shared
        let controller = CaptureShortcutController(defaults: UserDefaults(suiteName: UUID().uuidString)!, registrar: TestShortcutRegistrar())
        controller.start(action: {})
        let view = NSHostingView(rootView: CaptureShortcutSettings(shortcut: controller))
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 480, height: 340), styleMask: [.titled], backing: .buffered, defer: false)
        window.contentView = view
        window.orderFrontRegardless()
        defer { window.orderOut(nil) }
        RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        view.layoutSubtreeIfNeeded()
        let screenshot = Process()
        screenshot.executableURL = URL(fileURLWithPath: "/usr/sbin/screencapture")
        screenshot.arguments = ["-x", "-l", String(window.windowNumber), path]
        try screenshot.run()
        screenshot.waitUntilExit()
        XCTAssertEqual(screenshot.terminationStatus, 0)
    }
}

@MainActor private final class TestShortcutRegistrar: CaptureHotKeyRegistering {
    var onInvoke: (() -> Void)?
    var current: CaptureShortcut?
    var reject = false
    func replace(with shortcut: CaptureShortcut?) throws {
        if reject { throw CaptureShortcutError.registration(OSStatus(eventHotKeyExistsErr)) }
        current = shortcut
    }
}
