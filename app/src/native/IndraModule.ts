import {NativeModules, NativeEventEmitter, Platform} from 'react-native';

const {IndraModule: NativeIndra} = NativeModules;

if (!NativeIndra) {
  throw new Error(
    'IndraModule native module not found. Did you link the gomobile .aar/.xcframework?',
  );
}

const emitter = new NativeEventEmitter(NativeIndra);

export interface InboundMessage {
  id: string;
  conversation_id: string;
  sender_id: string;
  text: string;
  sent_at_unix: number;
  direction: 'inbound' | 'outbound';
}

export interface Conversation {
  id: string;
  is_group: boolean;
  name: string;
  participants: string[];
  unread_count: number;
}

export interface WhoamiResult {
  peer_id: string;
  box_pubkey: string;
  pqc_pubkey: string;
}

export type MessageListener = (message: InboundMessage) => void;

class Indra {
  private listeners: MessageListener[] = [];

  constructor() {
    emitter.addListener('onIndraMessage', (event: {json: string}) => {
      try {
        const msg: InboundMessage = JSON.parse(event.json);
        this.listeners.forEach(fn => fn(msg));
      } catch {
        // ignore malformed JSON
      }
    });
  }

  async start(
    dataDir: string,
    listenAddr: string = '',
    bootstrapPeer: string = '',
  ): Promise<void> {
    return NativeIndra.start(dataDir, listenAddr, bootstrapPeer);
  }

  async stop(): Promise<void> {
    return NativeIndra.stop();
  }

  async whoami(): Promise<WhoamiResult> {
    const json: string = await NativeIndra.whoami();
    return JSON.parse(json);
  }

  async peerID(): Promise<string> {
    return NativeIndra.peerID();
  }

  async addContact(
    peerID: string,
    pubkeyHex: string,
    alias: string,
  ): Promise<void> {
    return NativeIndra.addContact(peerID, pubkeyHex, alias);
  }

  async addContactPQC(
    peerID: string,
    pubkeyHex: string,
    alias: string,
    pqcPubkeyHex: string,
  ): Promise<void> {
    return NativeIndra.addContactPQC(peerID, pubkeyHex, alias, pqcPubkeyHex);
  }

  async parseAndAddContact(whoamiJson: string, alias: string): Promise<void> {
    return NativeIndra.parseAndAddContact(whoamiJson, alias);
  }

  async sendMessage(peerID: string, text: string): Promise<void> {
    return NativeIndra.sendMessage(peerID, text);
  }

  async sendGroupMessage(groupID: string, text: string): Promise<void> {
    return NativeIndra.sendGroupMessage(groupID, text);
  }

  async createGroup(
    name: string,
    memberPeerIDs: string[],
  ): Promise<string> {
    return NativeIndra.createGroup(name, memberPeerIDs.join(','));
  }

  async getConversations(): Promise<Conversation[]> {
    const json: string = await NativeIndra.getConversations();
    return JSON.parse(json);
  }

  async getMessages(
    conversationID: string,
    limit: number = 100,
  ): Promise<InboundMessage[]> {
    const json: string = await NativeIndra.getMessages(conversationID, limit);
    return JSON.parse(json);
  }

  async getAddrs(): Promise<string[]> {
    const json: string = await NativeIndra.getAddrs();
    return JSON.parse(json);
  }

  async setRelayURL(url: string): Promise<void> {
    return NativeIndra.setRelayURL(url);
  }

  async registerPushToken(token: string, platform: string = 'ios'): Promise<void> {
    return NativeIndra.registerPushToken(token, platform);
  }

  async fetchMailbox(): Promise<void> {
    return NativeIndra.fetchMailbox();
  }

  onMessage(listener: MessageListener): () => void {
    this.listeners.push(listener);
    return () => {
      this.listeners = this.listeners.filter(fn => fn !== listener);
    };
  }
}

export const indra = new Indra();
export default indra;
