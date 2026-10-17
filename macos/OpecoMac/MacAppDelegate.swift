import AppKit
import OSLog
import UserNotifications

final class MacAppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    private let logger = Logger(subsystem: "link.opeco.app", category: "Lifecycle")

    func applicationDidFinishLaunching(_ notification: Notification) {
        let info = Bundle.main.infoDictionary!
        let version = info["CFBundleShortVersionString"] as! String
        let build = info["CFBundleVersion"] as! String
        logger.notice("Launched \(version, privacy: .public) (\(build, privacy: .public))")
        UNUserNotificationCenter.current().delegate = self
        MacRuntime.shared.start()
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        // A quit Apple Event (from loginwindow, an updater, or another app) names its sender;
        // the in-app Quit button arrives without one.
        if let pid = NSAppleEventManager.shared().currentAppleEvent?
            .attributeDescriptor(forKeyword: keySenderPIDAttr)?.int32Value {
            var path = [CChar](repeating: 0, count: 4 * Int(MAXPATHLEN))
            let requester = proc_pidpath(pid, &path, UInt32(path.count)) > 0
                ? String(cString: path) : "path unavailable: \(String(cString: strerror(errno)))"
            logger.notice("Quit requested by pid \(pid) (\(requester, privacy: .public))")
        } else {
            logger.notice("Quit requested without an Apple Event")
        }
        return .terminateNow
    }

    func applicationWillTerminate(_ notification: Notification) {
        logger.notice("Terminating")
    }

    func application(_ application: NSApplication, open urls: [URL]) {
        for url in urls {
            MacRuntime.shared.open(url)
        }
    }

    func application(
        _ application: NSApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        PushCoordinator.shared.received(deviceToken: deviceToken)
    }

    func application(
        _ application: NSApplication,
        didFailToRegisterForRemoteNotificationsWithError error: Error
    ) {
        PushCoordinator.shared.failed(error)
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        if notification.request.content.userInfo["kind"] as? String == "status" { return [] }
        return [.banner, .sound]
    }
}
