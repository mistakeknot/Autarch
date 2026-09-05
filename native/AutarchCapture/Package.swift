// swift-tools-version: 6.2
import PackageDescription
let package = Package(name: "AutarchCapture", platforms: [.macOS(.v26)], products: [.executable(name: "AutarchCapture", targets: ["AutarchCapture"])], targets: [.executableTarget(name: "AutarchCapture"), .testTarget(name: "AutarchCaptureTests", dependencies: ["AutarchCapture"])], swiftLanguageModes: [.v5])
