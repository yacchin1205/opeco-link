import CoreImage.CIFilterBuiltins
import PhotosUI
import SwiftUI
import UIKit

struct ContentView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.scenePhase) private var scenePhase
    @State private var showingJoin = false
    @State private var showingDeviceManagement = false

    var body: some View {
        NavigationStack {
            Group {
                if let startupErrorMessage = model.startupErrorMessage {
                    StartupErrorView(message: startupErrorMessage)
                } else if !model.isReady {
                    StartupView()
                } else {
                    GeometryReader { geometry in
                        ScrollView {
                            LazyVStack(spacing: 16) {
#if DEBUG
                                if model.isAppBadgeUITest {
                                    Button("Enable notifications for UI test") {
                                        Task { await model.enableNotifications() }
                                    }
                                }
#endif
                                if model.isAwaitingDeviceApproval {
                                    DeviceApprovalWaitingCard()
                                }
                                if model.sessions.isEmpty {
                                    ContentUnavailableView {
                                        VStack(spacing: 8) {
                                            Image("OpecoEmpty")
                                                .resizable()
                                                .renderingMode(.template)
                                                .scaledToFit()
                                                .foregroundStyle(Color(red: 0.533, green: 0.533, blue: 0.533))
                                                .frame(height: 128)
                                                .accessibilityLabel("opeco")
                                                .accessibilityIdentifier("opeco-empty-outline")
                                            Text("No sessions")
                                        }
                                    } description: {
                                        Text("Scan the one-shot QR code shown by opeco.")
                                    } actions: {
                                        Button("Scan QR code") { showingJoin = true }
                                            .buttonStyle(.borderedProminent)
                                            .accessibilityIdentifier("empty-scan-qr-code")
                                    }
                                } else {
                                    LazyVGrid(columns: sessionColumns(for: geometry.size), alignment: .leading, spacing: 16) {
                                        ForEach(model.sessions) { session in
                                            SessionCard(session: session)
                                        }
                                    }
                                }
                            }
                            .padding()
                        }
                        .refreshable { await model.sync() }
                    }
                }
            }
            .background(Color.brandBackground.ignoresSafeArea())
            .toolbar(model.isReady ? .visible : .hidden, for: .navigationBar)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Label(model.connectionState.label, systemImage: connectionSymbol)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showingDeviceManagement = true
                    } label: {
                        Image(systemName: "macbook.and.iphone")
                            .font(.system(size: 18, weight: .medium))
                            .symbolRenderingMode(.monochrome)
                            .foregroundStyle(Color(red: 0.22, green: 0.70, blue: 0.92))
                            .opacity(1)
                            .frame(width: 24, height: 24)
                            .overlay(alignment: .topTrailing) {
                                if model.deviceCount > 1 {
                                    Text("\(model.deviceCount)")
                                        .font(.system(size: 9, weight: .bold, design: .rounded).monospacedDigit())
                                        .foregroundStyle(Color.white)
                                        .frame(width: 14, height: 14)
                                        .background(Color(red: 0.42, green: 0.42, blue: 0.45), in: Capsule())
                                        .opacity(1)
                                        .offset(x: 5, y: -5)
                                        .accessibilityHidden(true)
                                }
                            }
                    }
                    .buttonBorderShape(.circle)
                    .accessibilityLabel("Manage group")
                    .accessibilityValue(model.deviceCount > 1 ? "\(model.deviceCount) devices" : "1 device")
                }
                if #available(iOS 26.0, *) {
                    ToolbarSpacer(.fixed, placement: .topBarTrailing)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Scan QR code", systemImage: "qrcode.viewfinder") { showingJoin = true }
                        .disabled(!model.isReady)
                }
            }
            .sheet(isPresented: $showingJoin) {
                JoinSessionView(isPresented: $showingJoin)
            }
            .sheet(isPresented: $showingDeviceManagement) {
                DeviceManagementView(isPresented: $showingDeviceManagement)
            }
            .alert(
                "Add a device to this group?",
                isPresented: deviceAdditionApprovalPresented
            ) {
                Button("Add device") { model.confirmDeviceAddition() }
                Button("Cancel", role: .cancel) { model.cancelDeviceAddition() }
            } message: {
                Text("The new device will receive notifications and can respond as a member of this device group.")
            }
            .safeAreaInset(edge: .bottom) { OperationErrorView() }
            .alert("opeco", isPresented: noticePresented) {
                Button("OK") { model.dismissNotice() }
            } message: {
                Text(model.noticeMessage ?? "")
            }
            .task { await model.start() }
            .task(id: model.isReady) {
                guard model.isReady else { return }
                await model.runSyncLoop()
            }
            .onChange(of: scenePhase) { _, phase in
                guard phase == .active else { return }
                Task {
                    await model.sync()
                    await model.resumeNotifications()
                }
            }
        }
        .tint(.brandAccent)
    }

    private var deviceAdditionApprovalPresented: Binding<Bool> {
        Binding(
            get: { model.isDeviceAdditionApprovalPending },
            set: { if !$0 { model.cancelDeviceAddition() } }
        )
    }

    private var noticePresented: Binding<Bool> {
        Binding(
            get: { model.noticeMessage != nil },
            set: { if !$0 { model.dismissNotice() } }
        )
    }

    private var connectionSymbol: String {
        switch model.connectionState {
        case .preparing: "circle.dotted"
        case .syncing: "arrow.triangle.2.circlepath"
        case .current: "checkmark.circle"
        case .failed: "exclamationmark.triangle"
        }
    }

    private func sessionColumns(for size: CGSize) -> [GridItem] {
        Array(
            repeating: GridItem(.flexible(), spacing: 16, alignment: .top),
            count: SessionGridLayout.columnCount(idiom: UIDevice.current.userInterfaceIdiom, size: size)
        )
    }
}

enum SessionGridLayout {
    static func columnCount(idiom: UIUserInterfaceIdiom, size: CGSize) -> Int {
        guard idiom == .pad, size.width >= 600 else { return 1 }
        return size.width > size.height ? 3 : 2
    }
}

struct OperationErrorView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        if let message = model.errorMessage {
            VStack(alignment: .leading, spacing: 8) {
                Label("opeco error", systemImage: "exclamationmark.triangle")
                    .font(.headline)
                ScrollView {
                    Text(message)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .textSelection(.enabled)
                        .accessibilityIdentifier("operation-error-message")
                }
                .frame(maxHeight: 120)
                Button("Dismiss error") { model.dismissError() }
            }
            .padding()
            .background(.regularMaterial)
        }
    }
}

private struct StartupView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        VStack(spacing: 20) {
            Text("opeco")
                .font(.largeTitle.weight(.semibold))
                .accessibilityIdentifier("startup-title")
            ProgressView()
                .tint(.brandAccent)
                .accessibilityLabel("Preparing secure storage")
                .accessibilityIdentifier("startup-progress")
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .contentShape(Rectangle())
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("startup-screen")
#if DEBUG
        .onTapGesture { model.completeStartupUITest() }
        .accessibilityValue(colorScheme == .dark ? "dark" : "light")
#endif
    }
}

private struct StartupErrorView: View {
    @EnvironmentObject private var model: AppModel
    let message: String

    var body: some View {
        ContentUnavailableView {
            Label("Unable to start", systemImage: "exclamationmark.triangle")
        } description: {
            Text(message)
        } actions: {
            if model.canResetLocalData {
                Button("Erase saved data", role: .destructive) {
                    Task { await model.resetLocalData() }
                }
                .buttonStyle(.bordered)
            }
        }
    }
}

private struct DeviceApprovalWaitingCard: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("DEVICE GROUP")
                .font(.caption2.weight(.bold))
                .tracking(1.5)
                .foregroundStyle(Color.brandAccent)
            Text("Waiting to be added to a group")
                .font(.headline)
            Text("On a device already in that group, scan this QR code.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            if let link = model.deviceRequestLink {
                InvitationQRCodeView(value: link)
                ShareLink(item: link) { Label("Share link", systemImage: "square.and.arrow.up") }
            }
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.background, in: RoundedRectangle(cornerRadius: 20, style: .continuous))
    }
}

private struct DeviceManagementView: View {
    @EnvironmentObject private var model: AppModel
    @Binding var isPresented: Bool
    @State private var showingLeaveConfirmation = false
    @State private var showingRequestConfirmation = false
    @State private var removalTarget: GroupDevice?

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    deviceSection
                    Divider()
                    requestSection
                    if model.isSharingAcrossDevices {
                        Divider()
                        Button("Remove this device from the group", role: .destructive) {
                            showingLeaveConfirmation = true
                        }
                        .buttonStyle(.bordered)
                        .tint(.red)
                    }
                }
                .padding()
            }
            .background(Color.brandBackground)
            .navigationTitle("Device Group")
            .safeAreaInset(edge: .bottom) { OperationErrorView() }
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Close", systemImage: "xmark") { isPresented = false }
                        .labelStyle(.iconOnly)
                        .buttonBorderShape(.circle)
                }
            }
        }
        .confirmationDialog(
            "Add this device to another group?",
            isPresented: $showingRequestConfirmation,
            titleVisibility: .visible
        ) {
            Button("Remove and continue", role: .destructive) {
                Task { await model.createDeviceRequest(discardingCurrentState: true) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This device will be removed from its current group, and its saved sessions will be deleted.")
        }
        .confirmationDialog(
            "Remove this device from the group?",
            isPresented: $showingLeaveConfirmation,
            titleVisibility: .visible
        ) {
            Button("Remove from group", role: .destructive) {
                Task {
                    await model.leaveDeviceGroup()
                    if !model.isSharingAcrossDevices { isPresented = false }
                }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This device will stop receiving notifications sent to this group. Its saved sessions will be deleted. Other devices are not affected.")
        }
        .confirmationDialog(
            "Remove the selected device from the group?",
            isPresented: Binding(
                get: { removalTarget != nil },
                set: { if !$0 { removalTarget = nil } }
            ),
            titleVisibility: .visible
        ) {
            if let target = removalTarget {
                Button("Remove device", role: .destructive) {
                    Task {
                        await model.removeDevice(target.deviceID)
                        removalTarget = nil
                    }
                }
            }
            Button("Cancel", role: .cancel) { removalTarget = nil }
        } message: {
            Text("The selected device will stop receiving notifications sent to this group.")
        }
    }

    @ViewBuilder
    private var requestSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("ADD THIS DEVICE TO ANOTHER GROUP")
                .font(.caption2.weight(.bold))
                .tracking(1.5)
                .foregroundStyle(Color.brandAccent)
            if let link = model.deviceRequestLink {
                Text("On a device already in that group, scan this QR code.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                InvitationQRCodeView(value: link)
                ShareLink(item: link) { Label("Share link", systemImage: "square.and.arrow.up") }
            } else {
                Text("Add this device to the same group as a device you already use.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                Button("Add this device to another group") {
                    if model.deviceRequestWouldDiscardCurrentState { showingRequestConfirmation = true }
                    else { Task { await model.createDeviceRequest() } }
                }
                .buttonStyle(.borderedProminent)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var deviceSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("GROUP DEVICES")
                .font(.caption2.weight(.bold))
                .tracking(1.5)
                .foregroundStyle(Color.brandAccent)
            ForEach(model.groupDevices) { device in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(device.deviceID == model.deviceID ? "This device" : "Device")
                        Text(device.deviceID.prefix(8))
                            .font(.caption.monospaced())
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    if device.deviceID != model.deviceID {
                        Button(role: .destructive) { removalTarget = device } label: {
                            Label("Remove", systemImage: "trash")
                                .labelStyle(.iconOnly)
                        }
                        .buttonStyle(.bordered)
                        .tint(.red)
                    }
                }
            }
            if let timestamp = model.currentKeyTimestamp {
                Text("Key \(Date(timeIntervalSince1970: Double(timestamp) / 1_000).formatted())")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

private struct InvitationQRCodeView: View {
    private static let maximumImageSide: CGFloat = 328

    let value: String

    var body: some View {
        Group {
            if let image = InvitationQRCode.image(for: value) {
                Image(uiImage: image)
                    .interpolation(.none)
                    .resizable()
                    .scaledToFit()
                    .accessibilityLabel("QR code for adding this device to a group")
            } else {
                ContentUnavailableView("QR code unavailable", systemImage: "qrcode")
            }
        }
        .frame(maxWidth: Self.maximumImageSide)
        .padding(16)
        .background(.white, in: RoundedRectangle(cornerRadius: 12, style: .continuous))
        .frame(maxWidth: .infinity, alignment: .center)
    }
}

enum InvitationQRCode {
    static func image(for value: String) -> UIImage? {
        guard !value.isEmpty else { return nil }

        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(value.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage else { return nil }

        let scaled = output.transformed(by: CGAffineTransform(scaleX: 10, y: 10))
        let context = CIContext()
        guard let image = context.createCGImage(scaled, from: scaled.extent) else { return nil }
        return UIImage(cgImage: image)
    }
}

enum OpecoSessionPalette: String, CaseIterable {
    case red
    case orange
    case yellow
    case green
    case cyan
    case blue
    case purple
    case pink

    var assetName: String {
        switch self {
        case .blue: "OpecoSession"
        default: "OpecoSession\(rawValue.capitalized)"
        }
    }

    static func nearest(toHex color: String?) -> Self {
        guard let color,
              color.range(of: #"^#[0-9a-fA-F]{6}$"#, options: .regularExpression) != nil,
              let value = UInt64(color.dropFirst(), radix: 16) else { return .blue }

        let uiColor = UIColor(
            red: CGFloat((value >> 16) & 0xff) / 255,
            green: CGFloat((value >> 8) & 0xff) / 255,
            blue: CGFloat(value & 0xff) / 255,
            alpha: 1
        )
        var hue = CGFloat.zero
        var saturation = CGFloat.zero
        guard uiColor.getHue(&hue, saturation: &saturation, brightness: nil, alpha: nil),
              saturation >= 0.08 else { return .blue }

        return allCases.min {
            circularDistance(from: hue, to: $0.hue) < circularDistance(from: hue, to: $1.hue)
        } ?? .blue
    }

    private var hue: CGFloat {
        switch self {
        case .red: 0 / 360
        case .orange: 30 / 360
        case .yellow: 55 / 360
        case .green: 120 / 360
        case .cyan: 185 / 360
        case .blue: 220 / 360
        case .purple: 285 / 360
        case .pink: 330 / 360
        }
    }

    private static func circularDistance(from lhs: CGFloat, to rhs: CGFloat) -> CGFloat {
        let distance = abs(lhs - rhs)
        return min(distance, 1 - distance)
    }
}

private struct SessionCard: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.colorScheme) private var colorScheme
    @ScaledMetric(relativeTo: .caption) private var opecoSize = 18.0
    let session: SessionRecord
    @State private var responding = false
    @State private var showingFeedback = false
    @State private var togglingAttention = false
    @State private var attentionChanges = 0

    var body: some View {
        HStack(alignment: .top, spacing: opecoSize * 0.2) {
            Image(opecoPalette.assetName)
                .resizable()
                .scaledToFit()
                .frame(width: opecoSize * 2.5, height: opecoSize * 2.5)
                .padding(.top, 15)
                .accessibilityLabel("opeco")
                .accessibilityIdentifier("session-opeco")
                .accessibilityValue(opecoPalette.rawValue)
                .accessibilityHint(session.attention ? "Long press to stop watching status updates" : "Long press to watch status updates")
                .accessibilityAction(named: session.attention ? "Stop watching status updates" : "Watch status updates") {
                    toggleAttention()
                }
                .onLongPressGesture { toggleAttention() }

            VStack(alignment: .leading, spacing: 14) {
                HStack(alignment: .firstTextBaseline) {
                    HStack(spacing: 6) {
                        Text(session.title)
                            .font(.title3.weight(.semibold))
                        if session.attention {
                            Image(systemName: "eye.fill")
                                .font(.caption2)
                                .foregroundStyle(Color.brandAccent)
                                .accessibilityLabel("Watching status updates")
                        }
                    }
                    Spacer()
                    VStack(alignment: .trailing, spacing: 3) {
                        if session.unresolvedCount > 0 {
                            Text("\(session.unresolvedCount)")
                                .font(.caption2.bold().monospacedDigit())
                                .foregroundStyle(.white)
                                .padding(.horizontal, 7)
                                .padding(.vertical, 3)
                                .background(Color.brandAccent, in: Capsule())
                                .accessibilityLabel(session.unresolvedAccessibilityLabel)
                        }
                        if let updatedAt = session.updatedAt {
                            RelativeTimeText(timestampMilliseconds: updatedAt)
                        }
                        Text(expiryLabel)
                            .font(.caption.monospacedDigit())
                            .foregroundStyle(.secondary)
                    }
                }

                if let syncError = model.sessionSyncErrors[session.sessionID] {
                    Label(syncError, systemImage: "exclamationmark.triangle.fill")
                        .font(.caption)
                        .foregroundStyle(.orange)
                        .accessibilityIdentifier("session-sync-error")
                }
                if !session.status.isEmpty {
                    Label(session.status, systemImage: "waveform.path.ecg")
                        .font(.subheadline.weight(.medium))
                }
                ForEach(session.notifications) { notification in
                    HStack(alignment: .top, spacing: 12) {
                        VStack(alignment: .leading, spacing: 3) {
                            Text(notification.message)
                                .font(.body)
                                .textSelection(.enabled)
                            if let createdAt = notification.createdAt {
                                RelativeTimeText(timestampMilliseconds: createdAt)
                            }
                        }
                        Spacer(minLength: 8)
                        Button("Dismiss notification", systemImage: "xmark") {
                            let sessionID = session.sessionID
                            let notificationID = notification.id
                            Task {
                                await model.dismissNotification(sessionID: sessionID, notificationID: notificationID)
                            }
                        }
                        .labelStyle(.iconOnly)
                        .buttonStyle(.plain)
                        .foregroundStyle(.secondary)
                    }
                }
                if let request = session.request {
                    Divider()
                    HStack(alignment: .top, spacing: 12) {
                        VStack(alignment: .leading, spacing: 3) {
                            Text(request.prompt)
                                .font(.headline)
                            if let createdAt = request.createdAt {
                                RelativeTimeText(timestampMilliseconds: createdAt)
                            }
                        }
                        Spacer(minLength: 8)
                        Button("Dismiss request", systemImage: "xmark") {
                            let sessionID = session.sessionID
                            let requestID = request.id
                            responding = true
                            Task {
                                await model.dismissRequest(sessionID: sessionID, requestID: requestID)
                                responding = false
                            }
                        }
                        .labelStyle(.iconOnly)
                        .buttonStyle(.plain)
                        .foregroundStyle(.secondary)
                        .disabled(responding)
                    }
                    ForEach(request.options) { option in
                        Button(option.label) {
                            let sessionID = session.sessionID
                            let requestID = request.id
                            let optionID = option.id
                            responding = true
                            Task {
                                await model.respond(sessionID: sessionID, requestID: requestID, optionID: optionID)
                                responding = false
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .disabled(responding)
                    }
                }
                HStack {
                    Spacer()
                    Button("Send a message", systemImage: "bubble.left") { showingFeedback = true }
                        .labelStyle(.iconOnly)
                        .accessibilityLabel("Send a message")
                        .buttonStyle(.bordered)
                }
            }
            .padding(.vertical, 18)
            .padding(.trailing, 18)
            .padding(.leading, 28)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                panelColor.opacity(colorScheme == .dark ? 0.35 : 0.75),
                in: SessionBubbleShape(tailY: 15 + opecoSize * 1.25)
            )
            .overlay {
                SessionBubbleShape(tailY: 15 + opecoSize * 1.25)
                    .stroke(session.attention ? Color.brandAccent : Color.primary.opacity(0.08), lineWidth: session.attention ? 2 : 1)
            }
            .shadow(color: session.attention ? Color.brandAccent.opacity(0.55) : .clear, radius: 14)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .sensoryFeedback(.success, trigger: attentionChanges)
        .sheet(isPresented: $showingFeedback) {
            FeedbackView(sessionID: session.sessionID, protocolVersion: session.protocolVersion, isPresented: $showingFeedback)
        }
    }

    private func toggleAttention() {
        guard !togglingAttention else { return }
        let sessionID = session.sessionID
        let attention = !session.attention
        togglingAttention = true
        Task {
            if await model.setAttention(sessionID: sessionID, attention: attention) {
                attentionChanges += 1
            }
            togglingAttention = false
        }
    }

    private var panelColor: Color {
        session.color.flatMap(Color.init(hex:)) ?? Color(uiColor: .systemBackground)
    }

    private var opecoPalette: OpecoSessionPalette {
        .nearest(toHex: session.color)
    }

    private var expiryLabel: String {
        let remaining = Double(session.expiresAt) / 1_000 - Date().timeIntervalSince1970
        guard remaining > 0 else { return "Checking expiry" }
        let hours = max(1, Int(ceil(remaining / 3_600)))
        return "~\(hours)h"
    }
}

private struct SessionBubbleShape: Shape {
    let tailY: CGFloat

    func path(in rect: CGRect) -> Path {
        let tailWidth = 10.0
        let tailHalfHeight = 8.0
        let resolvedTailY = min(tailY, rect.height / 2)
        let body = CGRect(x: tailWidth, y: 0, width: max(0, rect.width - tailWidth), height: rect.height)
        let corner = min(20.0, body.width / 2, body.height / 2)

        var path = Path()
        path.move(to: CGPoint(x: body.minX + corner, y: body.minY))
        path.addLine(to: CGPoint(x: body.maxX - corner, y: body.minY))
        path.addQuadCurve(
            to: CGPoint(x: body.maxX, y: body.minY + corner),
            control: CGPoint(x: body.maxX, y: body.minY)
        )
        path.addLine(to: CGPoint(x: body.maxX, y: body.maxY - corner))
        path.addQuadCurve(
            to: CGPoint(x: body.maxX - corner, y: body.maxY),
            control: CGPoint(x: body.maxX, y: body.maxY)
        )
        path.addLine(to: CGPoint(x: body.minX + corner, y: body.maxY))
        path.addQuadCurve(
            to: CGPoint(x: body.minX, y: body.maxY - corner),
            control: CGPoint(x: body.minX, y: body.maxY)
        )
        path.addLine(to: CGPoint(x: body.minX, y: resolvedTailY + tailHalfHeight))
        path.addLine(to: CGPoint(x: rect.minX, y: resolvedTailY))
        path.addLine(to: CGPoint(x: body.minX, y: resolvedTailY - tailHalfHeight))
        path.addLine(to: CGPoint(x: body.minX, y: body.minY + corner))
        path.addQuadCurve(
            to: CGPoint(x: body.minX + corner, y: body.minY),
            control: CGPoint(x: body.minX, y: body.minY)
        )
        path.closeSubpath()
        return path
    }
}

private struct RelativeTimeText: View {
    let timestampMilliseconds: Int64

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            Text(RelativeTime.label(timestampMilliseconds: timestampMilliseconds, now: context.date))
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
        }
    }
}

private struct FeedbackView: View {
    @EnvironmentObject private var model: AppModel
    let sessionID: String
    let protocolVersion: Int
    @Binding var isPresented: Bool
    @State private var message = ""
    @State private var sending = false
    @State private var showingCamera = false
    @State private var selectedPhotoItems: [PhotosPickerItem] = []
    @State private var preparingPhoto = false
    @State private var photos: [PreparedPhoto] = []
    @State private var previewPhoto: PreparedPhoto?
    @State private var photoError: String?
    @State private var resultUnknown = false

    var body: some View {
        NavigationStack {
            VStack(spacing: 16) {
                TextEditor(text: $message)
                    .frame(minHeight: 120, maxHeight: 220)
                    .accessibilityLabel("Message")
                if protocolVersion == 4 {
                    ScrollView(.horizontal) {
                        HStack {
                            ForEach(photos.indices, id: \.self) { index in
                                VStack {
                                    Text("Photo \(index + 1)").font(.caption)
                                    if let image = UIImage(data: photos[index].jpeg) {
                                        Image(uiImage: image).resizable().scaledToFit().frame(width: 120, height: 120)
                                            .accessibilityLabel("Photo \(index + 1)")
                                            .accessibilityIdentifier("selected-photo-preview")
                                            .onTapGesture { previewPhoto = photos[index] }
                                            .accessibilityAddTraits(.isButton)
                                    }
                                    Button("Remove photo \(index + 1)", role: .destructive) { photos.remove(at: index) }
                                        .disabled(preparingPhoto || sending)
                                }
                            }
                        }
                    }
                    .frame(height: photos.isEmpty ? 0 : 180)
                    HStack {
                        Button("Take Photo", systemImage: "camera") { showingCamera = true }
                            .disabled(preparingPhoto || photos.count >= 5 || !UIImagePickerController.isSourceTypeAvailable(.camera))
                        PhotosPicker(selection: $selectedPhotoItems, maxSelectionCount: max(1, 5 - photos.count), selectionBehavior: .ordered, matching: .images) {
                            Label("Choose Photos", systemImage: "photo.on.rectangle")
                        }
                        .disabled(preparingPhoto || photos.count >= 5)
                    }
                    if preparingPhoto { ProgressView("Preparing photo") }
                    if let photoError {
                        Text(photoError)
                            .font(.caption)
                            .foregroundStyle(.red)
                            .accessibilityIdentifier("photo-preparation-error")
                    }
                }
            }
            .padding()
            .navigationTitle("Send a message")
            .safeAreaInset(edge: .bottom) { OperationErrorView() }
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { isPresented = false }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Send") {
                        let sessionID = sessionID
                        let message = message
                        let photos = photos
                        sending = true
                        Task {
                            let result = await model.sendFeedback(sessionID: sessionID, message: message, photos: photos)
                            if result == .sent { isPresented = false }
                            resultUnknown = result == .unknown
                            sending = false
                        }
                    }
                    .disabled(
                        sending || preparingPhoto || resultUnknown ||
                        (message.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && photos.isEmpty)
                    )
                }
            }
        }
        .onChange(of: selectedPhotoItems) { _, items in
            guard !items.isEmpty else { return }
            preparingPhoto = true
            photoError = nil
            Task {
                do {
                    var preparedPhotos: [PreparedPhoto] = []
                    for item in items {
                        guard let data = try await item.loadTransferable(type: Data.self),
                              let image = UIImage(data: data), let prepared = normalizedPhoto(image) else {
                            throw PhotoPreparationError.failed
                        }
                        preparedPhotos.append(prepared)
                    }
                    photos.append(contentsOf: preparedPhotos)
                } catch { photoError = error.localizedDescription }
                selectedPhotoItems = []
                preparingPhoto = false
            }
        }
        .sheet(isPresented: Binding(get: { previewPhoto != nil }, set: { if !$0 { previewPhoto = nil } })) {
            VStack {
                if let photo = previewPhoto, let image = UIImage(data: photo.jpeg) {
                    Image(uiImage: image).resizable().scaledToFit()
                }
                Button("Close") { previewPhoto = nil }
            }.padding()
        }
        .fullScreenCover(isPresented: $showingCamera) {
            CameraPicker { image in
                if let image {
                    if let prepared = normalizedPhoto(image) {
                        photos.append(prepared)
                        photoError = nil
                    } else { photoError = "The photo could not be prepared within the attachment limit." }
                }
                showingCamera = false
            }
            .ignoresSafeArea()
        }
    }
}

private enum PhotoPreparationError: LocalizedError {
    case failed

    var errorDescription: String? { "The selected photo could not be decoded or prepared within the attachment limit." }
}

private struct CameraPicker: UIViewControllerRepresentable {
    let completion: (UIImage?) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(completion: completion) }

    func makeUIViewController(context: Context) -> UIImagePickerController {
        let controller = UIImagePickerController()
        controller.sourceType = .camera
        controller.cameraCaptureMode = .photo
        controller.delegate = context.coordinator
        return controller
    }

    func updateUIViewController(_ uiViewController: UIImagePickerController, context: Context) {}

    final class Coordinator: NSObject, UIImagePickerControllerDelegate, UINavigationControllerDelegate {
        let completion: (UIImage?) -> Void
        init(completion: @escaping (UIImage?) -> Void) { self.completion = completion }
        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) { completion(nil) }
        func imagePickerController(
            _ picker: UIImagePickerController,
            didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]
        ) {
            completion(info[.originalImage] as? UIImage)
        }
    }
}

private func normalizedPhoto(_ image: UIImage) -> PreparedPhoto? {
    let sourceWidth = image.cgImage?.width ?? Int(image.size.width * image.scale)
    let sourceHeight = image.cgImage?.height ?? Int(image.size.height * image.scale)
    guard sourceWidth > 0, sourceHeight > 0 else { return nil }
    let initialScale = min(1, 2048 / CGFloat(max(sourceWidth, sourceHeight)))
    var width = max(1, Int((CGFloat(sourceWidth) * initialScale).rounded()))
    var height = max(1, Int((CGFloat(sourceHeight) * initialScale).rounded()))
    var quality: CGFloat = 0.82
    for attempt in 0..<10 {
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        format.opaque = true
        let rendered = UIGraphicsImageRenderer(size: CGSize(width: width, height: height), format: format).image { _ in
            image.draw(in: CGRect(x: 0, y: 0, width: width, height: height))
        }
        guard let jpeg = rendered.jpegData(compressionQuality: quality) else { return nil }
        if jpeg.count <= 1024 * 1024 || (attempt == 9 && jpeg.count <= 2 * 1024 * 1024 - 16) {
            return PreparedPhoto(jpeg: jpeg, width: width, height: height)
        }
        if quality > 0.55 {
            quality -= 0.09
        } else {
            width = max(1, Int((CGFloat(width) * 0.82).rounded()))
            height = max(1, Int((CGFloat(height) * 0.82).rounded()))
        }
    }
    return nil
}

private extension Color {
    static let brandBackground = Color("LaunchBackground")
    static let brandAccent = Color(red: 0.22, green: 0.70, blue: 0.92)

    init?(hex: String) {
        guard hex.range(of: #"^#[0-9a-fA-F]{6}$"#, options: .regularExpression) != nil,
              let value = UInt64(hex.dropFirst(), radix: 16) else { return nil }
        self.init(
            red: Double((value >> 16) & 0xff) / 255,
            green: Double((value >> 8) & 0xff) / 255,
            blue: Double(value & 0xff) / 255
        )
    }
}
