import AppKit
import ImageIO
import SwiftUI
import UniformTypeIdentifiers

struct MacMenuBarView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.openWindow) private var openWindow
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            content
            Divider()
            footer
        }
        .frame(width: 390)
    }

    private var header: some View {
        HStack {
            Label(model.connectionState.label, systemImage: connectionSymbol)
                .font(.caption)
                .foregroundStyle(.secondary)
            Spacer()
            Button {
                openWindow(id: "device-group")
                dismiss()
            } label: {
                Image(systemName: "macbook.and.iphone")
                    .font(.system(size: 15, weight: .medium))
                    .symbolRenderingMode(.monochrome)
                    .frame(width: 24, height: 24)
                    .overlay(alignment: .topTrailing) {
                        if model.deviceCount > 1 {
                            Text("\(model.deviceCount)")
                                .font(.system(size: 8, weight: .bold, design: .rounded).monospacedDigit())
                                .foregroundStyle(.white)
                                .frame(width: 13, height: 13)
                                .background(Color(red: 0.42, green: 0.42, blue: 0.45), in: Capsule())
                                .offset(x: 5, y: -5)
                                .accessibilityHidden(true)
                        }
                    }
            }
            .buttonStyle(.plain)
            .disabled(!model.isReady)
            .accessibilityLabel("Manage group")
            .accessibilityValue(model.deviceCount > 1 ? "\(model.deviceCount) devices" : "1 device")

            Button("Add Session", systemImage: "plus") {
                openWindow(id: "join-session")
            }
            .labelStyle(.iconOnly)
            .buttonStyle(.plain)
            .disabled(!model.isReady)

            Button("Refresh", systemImage: "arrow.clockwise") {
                Task { await model.sync() }
            }
            .labelStyle(.iconOnly)
            .buttonStyle(.plain)
            .disabled(!model.isReady)
        }
        .padding(14)
    }

    @ViewBuilder
    private var content: some View {
        if let startupError = model.startupErrorMessage {
            MacStartupErrorView(message: startupError)
                .padding(16)
        } else if !model.isReady {
            ProgressView("Preparing secure storage…")
                .frame(maxWidth: .infinity, minHeight: 180)
        } else {
            ScrollView {
                LazyVStack(spacing: 12) {
#if DEBUG
                    if model.isSyncRecoveryUITest {
                        Button("Restore test connection") {
                            CommandUITestTransport.relay.syncFailurePath = nil
                        }
                    }
#endif
                    if let error = model.sharedSyncError {
                        Label(error, systemImage: "exclamationmark.triangle")
                            .font(.caption)
                            .foregroundStyle(.orange)
                            .accessibilityIdentifier("shared-sync-error")
                    }
                    if let error = model.errorMessage {
                        MacErrorBanner(message: error)
                    }
                    if let notice = model.noticeMessage {
                        MacNoticeBanner(message: notice)
                    }
                    ForEach(model.sessions) { session in
                        MacSessionCard(session: session)
                    }
                    if model.sessions.isEmpty {
                        ContentUnavailableView {
                            VStack(spacing: 6) {
                                Image("OpecoEmpty")
                                    .resizable()
                                    .renderingMode(.template)
                                    .scaledToFit()
                                    .foregroundStyle(Color(red: 0.533, green: 0.533, blue: 0.533))
                                    .frame(height: 96)
                                    .accessibilityLabel("opeco")
                                    .accessibilityIdentifier("opeco-empty-outline")
                                Text("No sessions")
                            }
                        } description: {
                            Text("Open or paste a one-shot link shown by opeco.")
                        } actions: {
                            Button("Add Session") { openWindow(id: "join-session") }
                                .buttonStyle(.borderedProminent)
                        }
                        .frame(minHeight: 220)
                    }
                }
                .padding(12)
            }
            .frame(minHeight: 280, idealHeight: 480, maxHeight: 620)
        }
    }

    private var footer: some View {
        HStack {
            Spacer()
            Button("Quit") { NSApplication.shared.terminate(nil) }
        }
        .buttonStyle(.plain)
        .padding(14)
    }

    private var connectionSymbol: String {
        switch model.connectionState {
        case .preparing: "circle.dotted"
        case .syncing: "arrow.triangle.2.circlepath"
        case .current: "checkmark.circle"
        case .failed: "exclamationmark.triangle"
        }
    }
}

private struct MacStartupErrorView: View {
    @EnvironmentObject private var model: AppModel
    let message: String

    var body: some View {
        ContentUnavailableView {
            Label("Unable to start", systemImage: "exclamationmark.triangle")
        } description: {
            Text(message)
        } actions: {
            if model.canResetLocalData {
                Button("Erase Saved Data", role: .destructive) {
                    Task { await model.resetLocalData() }
                }
            }
        }
        .frame(maxWidth: .infinity, minHeight: 220)
    }
}

private struct MacErrorBanner: View {
    @EnvironmentObject private var model: AppModel
    let message: String

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.red)
            Text(message)
                .font(.callout)
                .textSelection(.enabled)
                .accessibilityIdentifier("error-message")
            Spacer(minLength: 4)
            Button("Dismiss Error", systemImage: "xmark") { model.dismissError() }
                .labelStyle(.iconOnly)
                .buttonStyle(.plain)
        }
        .padding(12)
        .background(.red.opacity(0.1), in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct MacNoticeBanner: View {
    @EnvironmentObject private var model: AppModel
    let message: String

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "info.circle.fill")
                .foregroundStyle(.blue)
            Text(message)
                .font(.callout)
            Spacer(minLength: 4)
            Button("Dismiss Notice", systemImage: "xmark") { model.dismissNotice() }
                .labelStyle(.iconOnly)
                .buttonStyle(.plain)
        }
        .padding(12)
        .background(.blue.opacity(0.1), in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct MacSessionCard: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.colorScheme) private var colorScheme
    @ScaledMetric(relativeTo: .caption) private var opecoSize = 18.0
    let session: SessionRecord
    @State private var responding = false
    @State private var composingMessage = false
    @State private var message = ""
    @State private var sendingMessage = false
    @State private var preparingPhoto = false
    @State private var photos: [PreparedPhoto] = []
    @State private var previewPhoto: PreparedPhoto?
    @State private var resultUnknown = false
    @State private var photoError: String?
    @State private var photoImportID = UUID()
    @State private var togglingAttention = false
    @FocusState private var messageFocused: Bool

    var body: some View {
        HStack(alignment: .top, spacing: opecoSize * 0.2) {
            Image(opecoPalette.assetName)
                .resizable()
                .scaledToFit()
                .frame(width: opecoSize * 2.5, height: opecoSize * 2.5)
                .padding(.top, 12)
                .accessibilityLabel("opeco")
                .accessibilityIdentifier("session-opeco")
                .accessibilityValue(opecoPalette.rawValue)
                .accessibilityHint(session.attention ? "Long press to stop watching status updates" : "Long press to watch status updates")
                .accessibilityAction(named: session.attention ? "Stop Watching Status Updates" : "Watch Status Updates") {
                    toggleAttention()
                }
                .onLongPressGesture { toggleAttention() }
                .contextMenu {
                    Button(session.attention ? "Stop Watching Status Updates" : "Watch Status Updates") {
                        toggleAttention()
                    }
                }

            VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .firstTextBaseline) {
                Text(session.title)
                    .font(.headline)
                if session.attention {
                    Image(systemName: "eye.fill")
                        .foregroundStyle(Color.opecoAccent)
                        .accessibilityLabel("Watching Status Updates")
                }
                Spacer()
                VStack(alignment: .trailing, spacing: 2) {
                    if session.unresolvedCount > 0 {
                        Text("\(session.unresolvedCount)")
                            .font(.caption2.bold().monospacedDigit())
                            .foregroundStyle(.white)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.opecoAccent, in: Capsule())
                            .accessibilityLabel(session.unresolvedAccessibilityLabel)
                    }
                    if let updatedAt = session.updatedAt {
                        MacRelativeTimeText(timestampMilliseconds: updatedAt)
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
                HStack(alignment: .top, spacing: 10) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(notification.message)
                            .font(.callout)
                            .textSelection(.enabled)
                        if let createdAt = notification.createdAt {
                            MacRelativeTimeText(timestampMilliseconds: createdAt)
                        }
                    }
                    Spacer(minLength: 4)
                    Button("Dismiss Notification", systemImage: "xmark") {
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
                HStack(alignment: .top, spacing: 10) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(request.prompt)
                            .font(.headline)
                        if let createdAt = request.createdAt {
                            MacRelativeTimeText(timestampMilliseconds: createdAt)
                        }
                    }
                    Spacer(minLength: 4)
                    Button("Dismiss Request", systemImage: "xmark") {
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
                    .disabled(responding)
                }
                HStack {
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
                        .disabled(responding)
                    }
                }
            }

            if composingMessage {
                VStack(alignment: .leading, spacing: 8) {
                    TextField("Message", text: $message)
                        .textFieldStyle(.roundedBorder)
                        .accessibilityIdentifier("mac-message-editor")
                        .focused($messageFocused)
                        .onSubmit { sendMessage() }
                    ScrollView(.horizontal) {
                        HStack {
                            ForEach(photos.indices, id: \.self) { index in
                                VStack {
                                    Text("Photo \(index + 1)").font(.caption)
                                    if let image = NSImage(data: photos[index].jpeg) {
                                        Image(nsImage: image).resizable().scaledToFit().frame(width: 88, height: 88)
                                            .accessibilityLabel("Photo \(index + 1)")
                                            .accessibilityIdentifier("mac-selected-photo-preview")
                                            .onTapGesture { previewPhoto = photos[index] }
                                            .accessibilityAddTraits(.isButton)
                                    }
                                    Button("Remove photo \(index + 1)", role: .destructive) { photos.remove(at: index) }
                                        .disabled(preparingPhoto || sendingMessage)
                                }
                            }
                        }
                    }
                    .frame(height: photos.isEmpty ? 0 : 145)
                    if let photoError {
                        Text(photoError)
                            .font(.caption)
                            .foregroundStyle(.red)
                            .accessibilityIdentifier("mac-photo-preparation-error")
                    }
                    HStack {
                        if session.protocolVersion == 4 {
                            PasteButton(supportedContentTypes: [.image]) { providers in
                                importPhoto(from: providers)
                            }
                            .labelStyle(.titleAndIcon)
                            .accessibilityLabel("Paste Image")
                            .disabled(preparingPhoto)
                        }
                        if preparingPhoto {
                            ProgressView()
                                .controlSize(.small)
                                .accessibilityLabel("Preparing image")
                        }
                        Spacer()
                        Button("Send") { sendMessage() }
                            .disabled(!canSendMessage)
                        Button("Cancel") { resetComposer() }
                    }
                }
                .background {
                    if session.protocolVersion == 4 {
                        MacImagePasteShortcut {
                            guard messageFocused else { return false }
                            return importPhotoFromPasteboard()
                        }
                            .frame(width: 0, height: 0)
                    }
                }
            } else {
                HStack {
                    Spacer()
                    Button("Send a Message", systemImage: "bubble.left") { composingMessage = true }
                        .labelStyle(.iconOnly)
                        .accessibilityLabel("Send a Message")
                }
            }
            }
            .padding(.vertical, 14)
            .padding(.trailing, 14)
            .padding(.leading, 24)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                panelColor.opacity(colorScheme == .dark ? 0.35 : 0.72),
                in: MacSessionBubbleShape(tailY: 12 + opecoSize * 1.25)
            )
            .overlay {
                MacSessionBubbleShape(tailY: 12 + opecoSize * 1.25)
                    .stroke(session.attention ? Color.opecoAccent : Color.primary.opacity(0.08), lineWidth: session.attention ? 2 : 1)
            }
            .shadow(color: session.attention ? Color.opecoAccent.opacity(0.55) : .clear, radius: 12)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .sheet(isPresented: Binding(get: { previewPhoto != nil }, set: { if !$0 { previewPhoto = nil } })) {
            VStack {
                if let photo = previewPhoto, let image = NSImage(data: photo.jpeg) {
                    Image(nsImage: image).resizable().scaledToFit()
                }
                Button("Close") { previewPhoto = nil }
            }.padding().frame(width: 600, height: 480)
        }
    }

    private func toggleAttention() {
        guard !togglingAttention else { return }
        let sessionID = session.sessionID
        let attention = !session.attention
        togglingAttention = true
        Task {
            _ = await model.setAttention(sessionID: sessionID, attention: attention)
            togglingAttention = false
        }
    }

    private func sendMessage() {
        guard canSendMessage else { return }
        let sessionID = session.sessionID
        let message = message
        let photos = photos
        sendingMessage = true
        Task {
            let result = await model.sendFeedback(sessionID: sessionID, message: message, photos: photos)
            resultUnknown = result == .unknown
            if result == .sent {
                resetComposer()
            }
            sendingMessage = false
        }
    }

    private var canSendMessage: Bool {
        !sendingMessage && !preparingPhoto && !resultUnknown &&
        (!message.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !photos.isEmpty)
    }

    private func resetComposer() {
        photoImportID = UUID()
        composingMessage = false
        messageFocused = false
        message = ""
        photos = []
        resultUnknown = false
        photoError = nil
        preparingPhoto = false
    }

    private func importPhoto(from providers: [NSItemProvider]) {
        guard !preparingPhoto else { return }
        guard !providers.isEmpty, photos.count + providers.count <= 5 else {
            photoError = "Select one to five images."
            return
        }
        let importID = UUID()
        photoImportID = importID
        preparingPhoto = true
        photoError = nil
        Task {
            do {
                var prepared: [PreparedPhoto] = []
                for provider in providers {
                    prepared.append(try await PhotoPreparer.load(provider))
                }
                guard photoImportID == importID, composingMessage else { return }
                photos.append(contentsOf: prepared)
            } catch {
                guard photoImportID == importID, composingMessage else { return }
                photoError = error.localizedDescription
            }
            preparingPhoto = false
        }
    }

    private func importPhotoFromPasteboard() -> Bool {
        guard !preparingPhoto else { return true }
        var imageItems: [NSItemProvider] = []
        for item in NSPasteboard.general.pasteboardItems ?? [] {
            guard let type = item.types.first(where: { UTType($0.rawValue)?.conforms(to: .image) == true }) else { continue }
            guard let data = item.data(forType: type) else {
                photoError = "The clipboard image format is not supported."
                return true
            }
            imageItems.append(NSItemProvider(item: data as NSData, typeIdentifier: type.rawValue))
        }
        guard !imageItems.isEmpty else { return false }
        importPhoto(from: imageItems)
        return true
    }

    private var panelColor: Color {
        session.color.flatMap(Color.init(hex:)) ?? Color(nsColor: .controlBackgroundColor)
    }

    private var opecoPalette: MacOpecoSessionPalette {
        .nearest(toHex: session.color)
    }

    private var expiryLabel: String {
        let remaining = Double(session.expiresAt) / 1_000 - Date().timeIntervalSince1970
        guard remaining > 0 else { return "Checking expiry" }
        return "~\(max(1, Int(ceil(remaining / 3_600))))h"
    }
}

private struct MacSessionBubbleShape: Shape {
    let tailY: CGFloat

    func path(in rect: CGRect) -> Path {
        let tailWidth = 10.0
        let tailHalfHeight = 8.0
        let resolvedTailY = min(tailY, rect.height / 2)
        let body = CGRect(x: tailWidth, y: 0, width: max(0, rect.width - tailWidth), height: rect.height)
        let corner = min(16.0, body.width / 2, body.height / 2)

        var path = Path()
        path.move(to: CGPoint(x: body.minX + corner, y: body.minY))
        path.addLine(to: CGPoint(x: body.maxX - corner, y: body.minY))
        path.addQuadCurve(to: CGPoint(x: body.maxX, y: body.minY + corner), control: CGPoint(x: body.maxX, y: body.minY))
        path.addLine(to: CGPoint(x: body.maxX, y: body.maxY - corner))
        path.addQuadCurve(to: CGPoint(x: body.maxX - corner, y: body.maxY), control: CGPoint(x: body.maxX, y: body.maxY))
        path.addLine(to: CGPoint(x: body.minX + corner, y: body.maxY))
        path.addQuadCurve(to: CGPoint(x: body.minX, y: body.maxY - corner), control: CGPoint(x: body.minX, y: body.maxY))
        path.addLine(to: CGPoint(x: body.minX, y: resolvedTailY + tailHalfHeight))
        path.addLine(to: CGPoint(x: rect.minX, y: resolvedTailY))
        path.addLine(to: CGPoint(x: body.minX, y: resolvedTailY - tailHalfHeight))
        path.addLine(to: CGPoint(x: body.minX, y: body.minY + corner))
        path.addQuadCurve(to: CGPoint(x: body.minX + corner, y: body.minY), control: CGPoint(x: body.minX, y: body.minY))
        path.closeSubpath()
        return path
    }
}

private enum MacOpecoSessionPalette: String, CaseIterable {
    case red, orange, yellow, green, cyan, blue, purple, pink

    var assetName: String {
        self == .blue ? "OpecoSession" : "OpecoSession\(rawValue.capitalized)"
    }

    static func nearest(toHex color: String?) -> Self {
        guard let color,
              color.range(of: #"^#[0-9a-fA-F]{6}$"#, options: .regularExpression) != nil,
              let value = UInt64(color.dropFirst(), radix: 16) else { return .blue }

        let red = CGFloat((value >> 16) & 0xff) / 255
        let green = CGFloat((value >> 8) & 0xff) / 255
        let blue = CGFloat(value & 0xff) / 255
        let maximum = max(red, green, blue)
        let minimum = min(red, green, blue)
        let delta = maximum - minimum
        guard maximum > 0, delta / maximum >= 0.08 else { return .blue }

        let hue: CGFloat
        if delta == 0 {
            hue = 0
        } else if maximum == red {
            hue = ((green - blue) / delta).truncatingRemainder(dividingBy: 6) / 6
        } else if maximum == green {
            hue = ((blue - red) / delta + 2) / 6
        } else {
            hue = ((red - green) / delta + 4) / 6
        }
        let normalizedHue = hue < 0 ? hue + 1 : hue
        return allCases.min {
            circularDistance(from: normalizedHue, to: $0.hue) < circularDistance(from: normalizedHue, to: $1.hue)
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

/// A composer-scoped local key monitor. It consumes Command-V only when the
/// action accepted an image, so ordinary text paste remains AppKit's behavior.
private struct MacImagePasteShortcut: NSViewRepresentable {
    let perform: () -> Bool

    func makeNSView(context: Context) -> MonitorView {
        MonitorView(perform: perform)
    }

    func updateNSView(_ nsView: MonitorView, context: Context) {
        nsView.perform = perform
    }

    static func dismantleNSView(_ nsView: MonitorView, coordinator: ()) {
        nsView.stopMonitoring()
    }

    final class MonitorView: NSView {
        var perform: () -> Bool
        private var monitor: Any?

        init(perform: @escaping () -> Bool) {
            self.perform = perform
            super.init(frame: .zero)
        }

        @available(*, unavailable)
        required init?(coder: NSCoder) {
            fatalError("init(coder:) has not been implemented")
        }

        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            if window == nil {
                stopMonitoring()
            } else {
                startMonitoring()
            }
        }

        func stopMonitoring() {
            guard let monitor else { return }
            NSEvent.removeMonitor(monitor)
            self.monitor = nil
        }

        private func startMonitoring() {
            guard monitor == nil else { return }
            monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
                let modifiers = event.modifierFlags.intersection(.deviceIndependentFlagsMask)
                guard modifiers == .command,
                      event.charactersIgnoringModifiers?.lowercased() == "v",
                      let self,
                      self.perform() else { return event }
                return nil
            }
        }

        deinit {
            stopMonitoring()
        }
    }
}


private struct MacRelativeTimeText: View {
    let timestampMilliseconds: Int64

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            let label = RelativeTime.label(timestampMilliseconds: timestampMilliseconds, now: context.date)
            Text(label)
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
                .accessibilityLabel(label)
        }
    }
}

extension Color {
    static let opecoAccent = Color(red: 0.22, green: 0.70, blue: 0.92)

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
