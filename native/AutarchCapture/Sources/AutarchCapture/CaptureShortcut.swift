import AppKit
import Carbon
import SwiftUI

struct CaptureShortcut: Codable, Equatable {
    var keyCode: UInt32
    var modifiers: UInt32
    static let suggested = CaptureShortcut(keyCode: UInt32(kVK_Space), modifiers: UInt32(controlKey | optionKey))
    static let modifierMask = UInt32(controlKey | optionKey | shiftKey | cmdKey)
    struct Key: Identifiable {
        let name: String
        let id: UInt32
    }
    static let keys: [Key] = [
        ("Space", kVK_Space),
        ("A", kVK_ANSI_A), ("B", kVK_ANSI_B), ("C", kVK_ANSI_C), ("D", kVK_ANSI_D),
        ("E", kVK_ANSI_E), ("F", kVK_ANSI_F), ("G", kVK_ANSI_G), ("H", kVK_ANSI_H),
        ("I", kVK_ANSI_I), ("J", kVK_ANSI_J), ("K", kVK_ANSI_K), ("L", kVK_ANSI_L),
        ("M", kVK_ANSI_M), ("N", kVK_ANSI_N), ("O", kVK_ANSI_O), ("P", kVK_ANSI_P),
        ("Q", kVK_ANSI_Q), ("R", kVK_ANSI_R), ("S", kVK_ANSI_S), ("T", kVK_ANSI_T),
        ("U", kVK_ANSI_U), ("V", kVK_ANSI_V), ("W", kVK_ANSI_W), ("X", kVK_ANSI_X),
        ("Y", kVK_ANSI_Y), ("Z", kVK_ANSI_Z),
        ("0", kVK_ANSI_0), ("1", kVK_ANSI_1), ("2", kVK_ANSI_2), ("3", kVK_ANSI_3),
        ("4", kVK_ANSI_4), ("5", kVK_ANSI_5), ("6", kVK_ANSI_6), ("7", kVK_ANSI_7),
        ("8", kVK_ANSI_8), ("9", kVK_ANSI_9),
        ("F1", kVK_F1), ("F2", kVK_F2), ("F3", kVK_F3), ("F4", kVK_F4),
        ("F5", kVK_F5), ("F6", kVK_F6), ("F7", kVK_F7), ("F8", kVK_F8),
        ("F9", kVK_F9), ("F10", kVK_F10), ("F11", kVK_F11), ("F12", kVK_F12),
    ].map { Key(name: $0.0, id: UInt32($0.1)) }

    var isValid: Bool {
        modifiers & ~Self.modifierMask == 0 && modifiers & UInt32(controlKey | cmdKey) != 0 && Self.keys.contains { $0.id == keyCode }
    }
    var label: String {
        [(controlKey, "⌃"), (optionKey, "⌥"), (shiftKey, "⇧"), (cmdKey, "⌘")]
            .filter { modifiers & UInt32($0.0) != 0 }.map { $0.1 }.joined()
            + (Self.keys.first { $0.id == keyCode }?.name ?? "Unknown key")
    }
}

enum CaptureShortcutError: LocalizedError {
    case registration(OSStatus)
    var errorDescription: String? {
        switch self {
        case .registration(let code) where code == eventHotKeyExistsErr:
            return "That shortcut is already in use. Choose another combination."
        case .registration(let code):
            return "macOS could not register this shortcut (\(code)). Choose another combination."
        }
    }
}

@MainActor protocol CaptureHotKeyRegistering: AnyObject {
    var onInvoke: (() -> Void)? { get set }
    // A failed replacement must retain the previous registration.
    func replace(with shortcut: CaptureShortcut?) throws
}

@MainActor final class CarbonCaptureHotKey: CaptureHotKeyRegistering {
    var onInvoke: (() -> Void)?
    private var handler: EventHandlerRef?
    private var hotKey: EventHotKeyRef?
    private var shortcut: CaptureShortcut?
    private var serial: UInt32 = 0
    var registrationID: UInt32 { serial }
    private static let signature: OSType = 0x41555256
    private static var nextRegistrationID: UInt32 = 0

    func replace(with newShortcut: CaptureShortcut?) throws {
        guard newShortcut != shortcut else { return }
        if handler == nil {
            var type = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
            let status = InstallEventHandler(GetApplicationEventTarget(), { _, event, pointer in
                guard let event, let pointer else { return OSStatus(eventNotHandledErr) }
                var id = EventHotKeyID()
                guard GetEventParameter(event, EventParamName(kEventParamDirectObject), EventParamType(typeEventHotKeyID), nil, MemoryLayout<EventHotKeyID>.size, nil, &id) == noErr else { return OSStatus(eventNotHandledErr) }
                let owner = Unmanaged<CarbonCaptureHotKey>.fromOpaque(pointer).takeUnretainedValue()
                let receivedID = id
                // Application-target Carbon keyboard callbacks run on the main loop.
                return MainActor.assumeIsolated {
                    guard receivedID.signature == CarbonCaptureHotKey.signature, receivedID.id == owner.serial, owner.hotKey != nil else { return OSStatus(eventNotHandledErr) }
                    owner.onInvoke?()
                    return noErr
                }
            }, 1, &type, Unmanaged.passUnretained(self).toOpaque(), &handler)
            guard status == noErr else { throw CaptureShortcutError.registration(status) }
        }
        var replacement: EventHotKeyRef?
        Self.nextRegistrationID &+= 1
        let nextSerial = Self.nextRegistrationID
        if let newShortcut {
            let status = RegisterEventHotKey(newShortcut.keyCode, newShortcut.modifiers,
                EventHotKeyID(signature: Self.signature, id: nextSerial), GetApplicationEventTarget(),
                OptionBits(kEventHotKeyExclusive), &replacement)
            guard status == noErr else { throw CaptureShortcutError.registration(status) }
        }
        if let hotKey {
            let status = UnregisterEventHotKey(hotKey)
            if status != noErr {
                if let replacement { UnregisterEventHotKey(replacement) }
                throw CaptureShortcutError.registration(status)
            }
        }
        hotKey = replacement; shortcut = newShortcut; serial = nextSerial
    }

    deinit {
        if let hotKey { UnregisterEventHotKey(hotKey) }
        if let handler { RemoveEventHandler(handler) }
    }
}

@MainActor final class CaptureShortcutController: ObservableObject {
    @Published private(set) var active: CaptureShortcut?
    @Published private(set) var error = ""
    private(set) var configured: CaptureShortcut?
    private let defaults: UserDefaults
    private let registrar: CaptureHotKeyRegistering
    private var started = false
    private static let preferenceKey = "reviewCaptureShortcut.v1"
    private struct Preference: Codable { let shortcut: CaptureShortcut? }

    init(defaults: UserDefaults = .standard, registrar: CaptureHotKeyRegistering? = nil) {
        self.defaults = defaults
        self.registrar = registrar ?? CarbonCaptureHotKey()
        if let data = defaults.data(forKey: Self.preferenceKey) {
            configured = (try? JSONDecoder().decode(Preference.self, from: data))?.shortcut
        } else { configured = .suggested }
    }
    func start(action: @escaping () -> Void) {
        guard !started else { return }; started = true
        registrar.onInvoke = action
        _ = apply(configured)
    }
    @discardableResult func apply(_ shortcut: CaptureShortcut?) -> Bool {
        guard shortcut?.isValid != false else {
            error = "Choose a listed key with Control or Command to keep ordinary typing available."
            return false
        }
        do {
            let data = try JSONEncoder().encode(Preference(shortcut: shortcut))
            try registrar.replace(with: shortcut)
            configured = shortcut; active = shortcut; error = ""
            defaults.set(data, forKey: Self.preferenceKey)
            return true
        } catch { self.error = error.localizedDescription; return false }
    }
}

struct CaptureShortcutSettings: View {
    @ObservedObject var shortcut: CaptureShortcutController
    @State private var draft = CaptureShortcut.suggested
    private func modifier(_ value: Int) -> Binding<Bool> {
        Binding(get: { draft.modifiers & UInt32(value) != 0 }, set: { enabled in
            if enabled { draft.modifiers |= UInt32(value) } else { draft.modifiers &= ~UInt32(value) }
        })
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Capture shortcut").font(.title2)
            Text("Bring Autarch forward and capture the selected review window for a note.")
            Text("Active: \(shortcut.active?.label ?? "Disabled")").font(.headline)
            HStack {
                Toggle("Control", isOn: modifier(controlKey))
                Toggle("Option", isOn: modifier(optionKey))
                Toggle("Shift", isOn: modifier(shiftKey))
                Toggle("Command", isOn: modifier(cmdKey))
            }.toggleStyle(.checkbox)
            Picker("Key", selection: $draft.keyCode) {
                ForEach(CaptureShortcut.keys) { key in Text(key.name).tag(key.id) }
            }
            HStack {
                Button("Apply \(draft.label)") { shortcut.apply(draft) }.disabled(!draft.isValid)
                Button("Disable shortcut") { shortcut.apply(nil) }
            }
            if !shortcut.error.isEmpty { Text(shortcut.error).foregroundStyle(.red) }
            Text("Choose Control or Command plus a key. Changes take effect immediately and are saved on this Mac.").font(.caption).foregroundStyle(.secondary)
        }.padding(20).frame(width: 440).onAppear { draft = shortcut.configured ?? .suggested }
    }
}
