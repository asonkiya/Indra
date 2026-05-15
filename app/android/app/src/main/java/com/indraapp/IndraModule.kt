package com.indraapp

import com.facebook.react.bridge.*
import com.facebook.react.modules.core.DeviceEventManagerModule
import mobile.Mobile
import mobile.InboundHandler

class IndraModule(private val reactContext: ReactApplicationContext) :
    ReactContextBaseJavaModule(reactContext) {

    private var client: mobile.Client? = null

    override fun getName(): String = "IndraModule"

    @ReactMethod
    fun start(dataDir: String, listenAddr: String, bootstrapPeer: String, promise: Promise) {
        try {
            if (client == null) {
                client = Mobile.newClient(dataDir)
            }
            client!!.setInboundHandler(object : InboundHandler {
                override fun onMessage(jsonMessage: String) {
                    val params = Arguments.createMap().apply {
                        putString("json", jsonMessage)
                    }
                    reactContext
                        .getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter::class.java)
                        .emit("onIndraMessage", params)
                }
            })
            client!!.start(listenAddr, bootstrapPeer)
            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("START_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun stop(promise: Promise) {
        try {
            client?.stop()
            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("STOP_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun whoami(promise: Promise) {
        try {
            promise.resolve(client?.whoami() ?: "")
        } catch (e: Exception) {
            promise.reject("WHOAMI_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun peerID(promise: Promise) {
        try {
            promise.resolve(client?.peerID() ?: "")
        } catch (e: Exception) {
            promise.reject("PEER_ID_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun addContact(peerID: String, pubkeyHex: String, alias: String, promise: Promise) {
        try {
            client?.addContact(peerID, pubkeyHex, alias)
            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("ADD_CONTACT_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun sendMessage(peerID: String, text: String, promise: Promise) {
        try {
            client?.sendMessage(peerID, text)
            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("SEND_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun sendGroupMessage(groupID: String, text: String, promise: Promise) {
        try {
            client?.sendGroupMessage(groupID, text)
            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("SEND_GROUP_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun createGroup(name: String, memberPeerIDsCSV: String, promise: Promise) {
        try {
            val groupID = client?.createGroup(name, memberPeerIDsCSV)
            promise.resolve(groupID)
        } catch (e: Exception) {
            promise.reject("CREATE_GROUP_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun getConversations(promise: Promise) {
        try {
            promise.resolve(client?.getConversations() ?: "[]")
        } catch (e: Exception) {
            promise.reject("GET_CONVOS_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun getMessages(convID: String, limit: Int, promise: Promise) {
        try {
            promise.resolve(client?.getMessages(convID, limit.toLong()) ?: "[]")
        } catch (e: Exception) {
            promise.reject("GET_MSGS_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun getAddrs(promise: Promise) {
        try {
            promise.resolve(client?.addrs() ?: "[]")
        } catch (e: Exception) {
            promise.reject("GET_ADDRS_ERROR", e.message, e)
        }
    }

    @ReactMethod
    fun addListener(eventName: String) {
        // Required for RN event emitter
    }

    @ReactMethod
    fun removeListeners(count: Int) {
        // Required for RN event emitter
    }
}
