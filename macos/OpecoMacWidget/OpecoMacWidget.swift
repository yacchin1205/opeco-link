import SwiftUI
import WidgetKit

@main
struct OpecoMacWidgetBundle: WidgetBundle {
    var body: some Widget {
        OpecoWidget()
    }
}

struct OpecoWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: WidgetSnapshotConfiguration.kind, provider: OpecoTimelineProvider()) { entry in
            OpecoWidgetView(entry: entry)
                .containerBackground(.fill.tertiary, for: .widget)
                .privacySensitive()
                .widgetURL(URL(string: "opecolink://sessions")!)
        }
        .configurationDisplayName("opeco.link")
        .description("See current sessions and unresolved items.")
        .supportedFamilies([.systemMedium, .systemLarge])
    }
}

struct OpecoWidgetEntry: TimelineEntry {
    let date: Date
    let snapshot: WidgetSnapshot?
    let errorMessage: String?
}

struct OpecoTimelineProvider: TimelineProvider {
    func placeholder(in context: Context) -> OpecoWidgetEntry {
        OpecoWidgetEntry(date: .now, snapshot: .preview, errorMessage: nil)
    }

    func getSnapshot(in context: Context, completion: @escaping (OpecoWidgetEntry) -> Void) {
        completion(context.isPreview ? placeholder(in: context) : loadEntry(at: .now))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<OpecoWidgetEntry>) -> Void) {
        let now = Date()
        completion(Timeline(entries: [loadEntry(at: now)], policy: .after(now.addingTimeInterval(300))))
    }

    private func loadEntry(at date: Date) -> OpecoWidgetEntry {
        do {
            return OpecoWidgetEntry(date: date, snapshot: try WidgetSnapshotStore().load() ?? WidgetSnapshot(sessions: []), errorMessage: nil)
        } catch {
            return OpecoWidgetEntry(date: date, snapshot: nil, errorMessage: error.localizedDescription)
        }
    }
}

private struct OpecoWidgetView: View {
    @Environment(\.widgetFamily) private var family
    @Environment(\.widgetRenderingMode) private var renderingMode
    let entry: OpecoWidgetEntry

    var body: some View {
        if let errorMessage = entry.errorMessage {
            ContentUnavailableView {
                Label("Widget unavailable", systemImage: "exclamationmark.triangle")
            } description: {
                Text(errorMessage)
            }
        } else if let snapshot = entry.snapshot {
            let sessions = snapshot.activeSessions(at: entry.date)
            VStack(alignment: .leading, spacing: family == .systemMedium ? 6 : 10) {
                header(unresolvedCount: sessions.reduce(0) { $0 + $1.unresolvedCount })
                if sessions.isEmpty {
                    ContentUnavailableView {
                        VStack(spacing: 6) {
                            Image("OpecoEmpty")
                                .resizable()
                                .scaledToFit()
                                .foregroundStyle(.secondary)
                                .frame(width: family == .systemLarge ? 72 : 48,
                                       height: family == .systemLarge ? 72 : 48)
                            Text("No sessions")
                        }
                    } description: {
                        Text("Open opeco.link to connect a session.")
                    }
                } else {
                    ForEach(sessions.prefix(family == .systemLarge ? 4 : 2)) { session in
                        WidgetSessionRow(session: session, compact: family == .systemMedium)
                    }
                }
                Spacer(minLength: 0)
            }
        }
    }

    private func header(unresolvedCount: Int) -> some View {
        HStack {
            HStack(spacing: 6) {
                Image(renderingMode == .fullColor ? "OpecoSession" : "OpecoEmpty")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 22, height: 22)
                Text("opeco.link")
                    .font(.headline)
            }
            Spacer()
            if unresolvedCount > 0 {
                Text("\(unresolvedCount)")
                    .font(.caption.bold().monospacedDigit())
                    .foregroundStyle(.white)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 2)
                    .background(.cyan, in: Capsule())
                    .accessibilityLabel("\(unresolvedCount) unresolved items")
            }
        }
    }
}

private struct WidgetSessionRow: View {
    let session: WidgetSessionSnapshot
    let compact: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack {
                Text(session.title)
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                Spacer()
                if compact {
                    updatedTime
                }
                if session.unresolvedCount > 0 {
                    Text("\(session.unresolvedCount)")
                        .font(.caption2.bold().monospacedDigit())
                }
            }
            Label(session.summary, systemImage: session.itemKind.symbol)
                .font(.caption)
                .lineLimit(1)
            if !compact {
                updatedTime
            }
        }
        .padding(compact ? 6 : 8)
        .background(Color.secondary.opacity(0.12), in: RoundedRectangle(cornerRadius: 10))
    }

    private var updatedTime: some View {
        Text(Date(timeIntervalSince1970: Double(session.updatedAt) / 1_000), style: .relative)
            .font(.caption2.monospacedDigit())
            .foregroundStyle(.secondary)
            .lineLimit(1)
    }
}

private extension WidgetItemKind {
    var symbol: String {
        switch self {
        case .notification: "bell.fill"
        case .request: "questionmark.bubble.fill"
        case .status: "waveform.path.ecg"
        }
    }
}

private extension WidgetSnapshot {
    static let preview = WidgetSnapshot(sessions: [
        WidgetSessionSnapshot(
            id: "preview-request", title: "Release review", summary: "Deploy this build?", itemKind: .request,
            color: "#f2d7ee", unresolvedCount: 1,
            updatedAt: Int64(Date().addingTimeInterval(-120).timeIntervalSince1970 * 1_000),
            expiresAt: Int64(Date().addingTimeInterval(86_400).timeIntervalSince1970 * 1_000)
        ),
        WidgetSessionSnapshot(
            id: "preview-status", title: "Build pipeline", summary: "Tests passed", itemKind: .status,
            color: "#d6e4ff", unresolvedCount: 0,
            updatedAt: Int64(Date().addingTimeInterval(-600).timeIntervalSince1970 * 1_000),
            expiresAt: Int64(Date().addingTimeInterval(86_400).timeIntervalSince1970 * 1_000)
        ),
    ])
}
