import Foundation
import React
import Indra  // gomobile-generated framework

@objc(IndraModule)
class IndraModule: RCTEventEmitter {

  private var client: MobileClient?
  private var hasListeners = false
  private var pushObserver: NSObjectProtocol?

  override static func moduleName() -> String! {
    return "IndraModule"
  }

  @objc override static func requiresMainQueueSetup() -> Bool {
    return false
  }

  override func supportedEvents() -> [String]! {
    return ["onIndraMessage"]
  }

  override func startObserving() {
    hasListeners = true
    // Listen for silent push notifications to auto-fetch the mailbox.
    pushObserver = NotificationCenter.default.addObserver(
      forName: .didReceiveSilentPush,
      object: nil,
      queue: .main
    ) { [weak self] _ in
      self?.client?.fetchMailbox()
    }
  }

  override func stopObserving() {
    hasListeners = false
    if let obs = pushObserver {
      NotificationCenter.default.removeObserver(obs)
      pushObserver = nil
    }
  }

  @objc func start(
    _ dataDir: String,
    listenAddr: String,
    bootstrapPeer: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    do {
      if client == nil {
        // Resolve empty path to the app's sandboxed Documents directory so
        // each simulator/device gets its own isolated database.
        let resolvedDir: String
        if dataDir.isEmpty {
          let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first!
          resolvedDir = docs.appendingPathComponent("indra").path
        } else {
          resolvedDir = dataDir
        }
        var error: NSError?
        client = MobileNewClient(resolvedDir, &error)
        if let error = error {
          reject("START_ERROR", error.localizedDescription, error)
          return
        }
      }

      let handler = InboundBridge(module: self)
      client?.setInboundHandler(handler)

      try client?.start(listenAddr, bootstrapPeer: bootstrapPeer)
      resolve(nil)
    } catch {
      reject("START_ERROR", error.localizedDescription, error)
    }
  }

  @objc func stop(
    _ resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    client?.stop()
    resolve(nil)
  }

  @objc func whoami(
    _ resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    resolve(client?.whoami() ?? "")
  }

  @objc func peerID(
    _ resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    resolve(client?.peerID() ?? "")
  }

  @objc func addContact(
    _ peerID: String,
    pubkeyHex: String,
    alias: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    do {
      try client?.addContact(peerID, pubkeyHex: pubkeyHex, alias: alias)
      resolve(nil)
    } catch {
      reject("ADD_CONTACT_ERROR", error.localizedDescription, error)
    }
  }

  @objc func addContactPQC(
    _ peerID: String,
    pubkeyHex: String,
    alias: String,
    pqcPubkeyHex: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    do {
      try client?.addContactPQC(peerID, pubkeyHex: pubkeyHex, alias: alias, pqcPubkeyHex: pqcPubkeyHex)
      resolve(nil)
    } catch {
      reject("ADD_CONTACT_PQC_ERROR", error.localizedDescription, error)
    }
  }

  @objc func parseAndAddContact(
    _ whoamiStr: String,
    alias: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    do {
      try client?.parseAndAddContact(whoamiStr, alias: alias)
      resolve(nil)
    } catch {
      reject("PARSE_ADD_CONTACT_ERROR", error.localizedDescription, error)
    }
  }

  @objc func sendMessage(
    _ peerID: String,
    text: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    do {
      try client?.sendMessage(peerID, text: text)
      resolve(nil)
    } catch {
      reject("SEND_ERROR", error.localizedDescription, error)
    }
  }

  @objc func sendGroupMessage(
    _ groupID: String,
    text: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    do {
      try client?.sendGroupMessage(groupID, text: text)
      resolve(nil)
    } catch {
      reject("SEND_GROUP_ERROR", error.localizedDescription, error)
    }
  }

  @objc func createGroup(
    _ name: String,
    memberPeerIDsCSV: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    do {
      var error: NSError?
      let groupID = client?.createGroup(name, memberPeerIDsCSV: memberPeerIDsCSV, error: &error)
      if let error = error {
        reject("CREATE_GROUP_ERROR", error.localizedDescription, error)
        return
      }
      resolve(groupID)
    } catch {
      reject("CREATE_GROUP_ERROR", error.localizedDescription, error)
    }
  }

  @objc func getConversations(
    _ resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    resolve(client?.getConversations() ?? "[]")
  }

  @objc func getMessages(
    _ convID: String,
    limit: Int,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    resolve(client?.getMessages(convID, limit: limit) ?? "[]")
  }

  @objc func getAddrs(
    _ resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    resolve(client?.addrs() ?? "[]")
  }

  @objc func setRelayURL(
    _ url: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    client?.setRelayURL(url)
    resolve(nil)
  }

  @objc func registerPushToken(
    _ token: String,
    platform: String,
    resolver resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    do {
      try client?.registerPushToken(token, platform: platform)
      resolve(nil)
    } catch {
      reject("REGISTER_PUSH_ERROR", error.localizedDescription, error)
    }
  }

  @objc func fetchMailbox(
    _ resolve: @escaping RCTPromiseResolveBlock,
    rejecter reject: @escaping RCTPromiseRejectBlock
  ) {
    client?.fetchMailbox()
    resolve(nil)
  }

  func emitMessage(_ json: String) {
    guard hasListeners else { return }
    sendEvent(withName: "onIndraMessage", body: ["json": json])
  }
}

// InboundBridge implements the gomobile InboundHandler protocol.
class InboundBridge: NSObject, MobileInboundHandlerProtocol {
  weak var module: IndraModule?

  init(module: IndraModule) {
    self.module = module
  }

  func onMessage(_ jsonMessage: String?) {
    guard let json = jsonMessage else { return }
    module?.emitMessage(json)
  }
}
