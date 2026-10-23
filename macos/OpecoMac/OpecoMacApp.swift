import AppKit
import Combine
import OSLog
import ServiceManagement
import SwiftUI

@main
struct OpecoMacApp: App {
    @NSApplicationDelegateAdaptor(MacAppDelegate.self) private var appDelegate
    @ObservedObject private var model = MacRuntime.shared.model

    var body: some Scene {
        MenuBarExtra {
            MacMenuBarView()
                .environmentObject(model)
        } label: {
            MacMenuBarLabel()
                .environmentObject(model)
        }
        .menuBarExtraStyle(.window)

        Window("Add Session", id: "join-session") {
            MacJoinView()
                .environmentObject(model)
        }
        .windowResizability(.contentSize)

        Window("Device Group", id: "device-group") {
            MacDeviceGroupView()
                .environmentObject(model)
        }
        .defaultSize(width: 520, height: 620)

        Window("Sessions", id: "sessions") {
            MacMenuBarView(presentation: .window)
                .environmentObject(model)
                .handlesExternalEvents(
                    preferring: ["opecolink://sessions"],
                    allowing: ["opecolink://sessions"]
                )
        }
        .handlesExternalEvents(matching: ["opecolink://sessions"])
        .defaultSize(width: 1100, height: 720)

        Window("Add Device", id: "device-addition-approval") {
            MacDeviceAdditionApprovalView()
                .environmentObject(model)
        }
        .windowResizability(.contentSize)

        Settings {
            MacSettingsView()
                .environmentObject(model)
        }
    }
}

private struct MacSettingsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var loginItemStatus = SMAppService.mainApp.status

    var body: some View {
        Form {
            Toggle("Open at Login", isOn: Binding(get: { loginItemStatus == .enabled }, set: setOpenAtLogin))
                .accessibilityIdentifier("open-at-login")
            if loginItemStatus == .requiresApproval {
                Button("Allow in System Settings") { SMAppService.openSystemSettingsLoginItems() }
            }
            if let error = model.errorMessage {
                Text(error)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
                    .accessibilityIdentifier("settings-error")
            }
        }
        .formStyle(.grouped)
        .scrollDisabled(true)
        .fixedSize(horizontal: false, vertical: true)
        .frame(width: 440)
        // The person can also change this in System Settings.
        .onAppear { loginItemStatus = SMAppService.mainApp.status }
    }

    private func setOpenAtLogin(_ enabled: Bool) {
        do {
            if enabled {
                try SMAppService.mainApp.register()
            } else {
                try SMAppService.mainApp.unregister()
            }
        } catch {
            model.reportError("Open at Login could not be changed: \(error.localizedDescription)")
        }
        loginItemStatus = SMAppService.mainApp.status
    }
}

private struct MacMenuBarLabel: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.openWindow) private var openWindow
    @ObservedObject private var runtime = MacRuntime.shared

    var body: some View {
        let unresolvedCount = model.sessions.unresolvedCount
        let hasSyncError = model.connectionState == .failed
        HStack(spacing: 3) {
            Image("OpecoMenuBar")
                .resizable()
                .scaledToFit()
                .frame(width: 18, height: 18)
                .id("opeco-menu-bar-icon")
            if hasSyncError {
                Image(systemName: "exclamationmark.triangle.fill")
                    .id("sync-error-icon")
            }
            if unresolvedCount > 0 {
                Text("\(unresolvedCount)")
                    .monospacedDigit()
            }
        }
            .id(hasSyncError ? "sync-error-label" : "notification-label-\(unresolvedCount)")
            .accessibilityLabel(
                hasSyncError
                    ? "opeco, sync error"
                    : unresolvedCount == 0
                    ? "opeco, no unresolved items"
                    : "opeco, \(unresolvedCount) unresolved \(unresolvedCount == 1 ? "item" : "items")"
            )
            .onChange(of: model.isDeviceAdditionApprovalPending) { _, pending in
                if pending { presentDeviceAdditionApproval() }
            }
            .onAppear {
                if model.isDeviceAdditionApprovalPending { presentDeviceAdditionApproval() }
                presentSessionsWindowIfRequested()
            }
            .onChange(of: runtime.sessionsWindowRequest) { _, request in
                if request > 0 { presentSessionsWindowIfRequested() }
            }
    }

    private func presentDeviceAdditionApproval() {
        NSApp.activate(ignoringOtherApps: true)
        openWindow(id: "device-addition-approval")
    }

    private func presentSessionsWindowIfRequested() {
        guard runtime.sessionsWindowRequest > 0 else { return }
        NSApp.activate(ignoringOtherApps: true)
        openWindow(id: "sessions")
        runtime.didPresentSessionsWindow()
    }
}

private struct MacDeviceAdditionApprovalView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var approving = false

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Add a Device?")
                .font(.title2.weight(.semibold))
            Text("The new device will receive notifications and can respond as a member of this device group.")
                .foregroundStyle(.secondary)
            if let error = model.errorMessage {
                Text(error)
                    .font(.callout)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
                    .accessibilityIdentifier("device-addition-error")
            }
            HStack {
                Spacer()
                Button("Cancel") {
                    model.cancelDeviceAddition()
                    dismiss()
                }
                .disabled(approving)
                Button("Add Device") {
                    approving = true
                    Task {
                        if await model.approvePendingDeviceAddition() { dismiss() }
                        approving = false
                    }
                }
                .buttonStyle(.borderedProminent)
                .disabled(approving)
            }
        }
        .padding(24)
        .frame(width: 440)
        .onDisappear {
            if model.isDeviceAdditionApprovalPending { model.cancelDeviceAddition() }
        }
    }
}

@MainActor
final class MacRuntime: ObservableObject {
    static let shared = MacRuntime()

    let model: AppModel
    @Published private(set) var sessionsWindowRequest = 0

    private let widgetSnapshotCoordinator: WidgetSnapshotCoordinator
    private let logger = Logger(subsystem: "link.opeco.app", category: "Lifecycle")
    private var started = false

    private init() {
        let model = AppModel()
        self.model = model
        #if MAC_GUI_TESTING
        widgetSnapshotCoordinator = WidgetSnapshotCoordinator(
            model: model,
            store: WidgetSnapshotStore(directoryURL: FileManager.default.temporaryDirectory)
        )
        #else
        widgetSnapshotCoordinator = WidgetSnapshotCoordinator(model: model)
        #endif
    }

    func start() {
        guard !started else { return }
        started = true
        let notifications = NSWorkspace.shared.notificationCenter
        notifications.addObserver(self, selector: #selector(willSleep), name: NSWorkspace.willSleepNotification, object: nil)
        notifications.addObserver(self, selector: #selector(didWake), name: NSWorkspace.didWakeNotification, object: nil)
        model.setAutomaticSyncEnabled(true)
        Task {
            await model.start()
        }
    }

    @objc private func willSleep() {
        logger.notice("System will sleep; pausing sync")
        model.setAutomaticSyncEnabled(false)
    }

    @objc private func didWake() {
        logger.notice("System woke; resuming sync")
        model.setAutomaticSyncEnabled(true)
    }

    func open(_ url: URL) {
        start()
        if url.scheme == "opecolink" {
            guard url.host == "sessions", url.path.isEmpty else {
                model.reportError("This opeco link cannot be opened.")
                return
            }
            sessionsWindowRequest += 1
            return
        }
        Task { await model.openUniversalLink(url) }
    }

    func didPresentSessionsWindow() {
        sessionsWindowRequest = 0
    }
}
