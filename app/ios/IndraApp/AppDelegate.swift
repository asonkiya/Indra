import UIKit
import React
import React_RCTAppDelegate
import ReactAppDependencyProvider

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
  var window: UIWindow?

  var reactNativeDelegate: ReactNativeDelegate?
  var reactNativeFactory: RCTReactNativeFactory?

  func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
  ) -> Bool {
    let delegate = ReactNativeDelegate()
    let factory = RCTReactNativeFactory(delegate: delegate)
    delegate.dependencyProvider = RCTAppDependencyProvider()

    reactNativeDelegate = delegate
    reactNativeFactory = factory

    window = UIWindow(frame: UIScreen.main.bounds)

    factory.startReactNative(
      withModuleName: "IndraApp",
      in: window,
      launchOptions: launchOptions
    )

    // Register for remote (silent) push notifications.
    application.registerForRemoteNotifications()

    return true
  }

  // Called when APNs returns the device token.
  func application(
    _ application: UIApplication,
    didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
  ) {
    let token = deviceToken.map { String(format: "%02x", $0) }.joined()
    // Store the token so the React Native layer can read it.
    UserDefaults.standard.set(token, forKey: "apns_device_token")
    NotificationCenter.default.post(name: .didReceiveAPNsToken, object: nil, userInfo: ["token": token])
  }

  func application(
    _ application: UIApplication,
    didFailToRegisterForRemoteNotificationsWithError error: Error
  ) {
    print("Failed to register for remote notifications: \(error)")
  }

  // Handle silent push — wake the Indra node to fetch mailbox.
  func application(
    _ application: UIApplication,
    didReceiveRemoteNotification userInfo: [AnyHashable: Any],
    fetchCompletionHandler completionHandler: @escaping (UIBackgroundFetchResult) -> Void
  ) {
    // Emit an event so the React Native layer can call fetchMailbox().
    NotificationCenter.default.post(name: .didReceiveSilentPush, object: nil)
    // Give the node a few seconds to poll the DHT mailbox.
    DispatchQueue.main.asyncAfter(deadline: .now() + 25) {
      completionHandler(.newData)
    }
  }
}

extension Notification.Name {
  static let didReceiveAPNsToken = Notification.Name("didReceiveAPNsToken")
  static let didReceiveSilentPush = Notification.Name("didReceiveSilentPush")
}

class ReactNativeDelegate: RCTDefaultReactNativeFactoryDelegate {
  override func sourceURL(for bridge: RCTBridge) -> URL? {
    self.bundleURL()
  }

  override func bundleURL() -> URL? {
#if DEBUG
    RCTBundleURLProvider.sharedSettings().jsBundleURL(forBundleRoot: "index")
#else
    Bundle.main.url(forResource: "main", withExtension: "jsbundle")
#endif
  }
}
