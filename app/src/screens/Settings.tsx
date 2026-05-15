import React, {useCallback, useEffect, useState} from 'react';
import {ScrollView, StyleSheet, View} from 'react-native';
import {Button, Divider, List, Text, useTheme} from 'react-native-paper';
import {Clipboard} from 'react-native';
import QRCode from 'react-native-qrcode-svg';
import indra, {type WhoamiResult} from '../native/IndraModule';

export default function Settings() {
  const [peerID, setPeerID] = useState('');
  const [whoami, setWhoami] = useState<WhoamiResult | null>(null);
  const [whoamiRaw, setWhoamiRaw] = useState('');
  const [addrs, setAddrs] = useState<string[]>([]);
  const [copied, setCopied] = useState('');
  const theme = useTheme();

  const load = useCallback(async () => {
    try {
      const [pid, wai, addrList] = await Promise.all([
        indra.peerID(),
        indra.whoami(),
        indra.getAddrs(),
      ]);
      setPeerID(pid);
      setWhoami(wai);
      setWhoamiRaw(JSON.stringify(wai));
      setAddrs(addrList);
    } catch {
      // native module not ready
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const copyToClipboard = (label: string, value: string) => {
    Clipboard.setString(value);
    setCopied(label);
    setTimeout(() => setCopied(''), 2000);
  };

  const boxPubkey = whoami?.box_pubkey ?? '';
  const pqcPubkey = whoami?.pqc_pubkey ?? '';

  return (
    <ScrollView
      style={[styles.container, {backgroundColor: theme.colors.background}]}
      contentContainerStyle={styles.content}>
      <Text variant="headlineSmall" style={styles.heading}>
        Your Identity
      </Text>
      <Text variant="bodySmall" style={styles.hint}>
        Let others scan this QR code to add you as a contact.
      </Text>

      {whoamiRaw ? (
        <View style={styles.qrContainer}>
          <QRCode
            value={whoamiRaw}
            size={200}
            backgroundColor="white"
            color="black"
          />
          <Button
            mode="outlined"
            compact
            style={styles.qrCopyButton}
            onPress={() => copyToClipboard('Whoami', whoamiRaw)}>
            {copied === 'Whoami' ? 'Copied!' : 'Copy Whoami JSON'}
          </Button>
        </View>
      ) : (
        <Text style={styles.hint}>Generating QR code...</Text>
      )}

      <Divider style={styles.divider} />

      <List.Item
        title="Peer ID"
        description={peerID || 'Loading...'}
        descriptionNumberOfLines={2}
        right={() => (
          <Button
            compact
            onPress={() => copyToClipboard('Peer ID', peerID)}>
            {copied === 'Peer ID' ? 'Copied!' : 'Copy'}
          </Button>
        )}
      />

      <Divider />

      <List.Item
        title="Box Public Key"
        description={boxPubkey || 'Loading...'}
        descriptionNumberOfLines={2}
        right={() => (
          <Button
            compact
            onPress={() => copyToClipboard('Box Key', boxPubkey)}>
            {copied === 'Box Key' ? 'Copied!' : 'Copy'}
          </Button>
        )}
      />

      <Divider />

      <List.Item
        title="PQC Public Key (ML-KEM-768)"
        description={pqcPubkey ? pqcPubkey.slice(0, 32) + '...' : 'Loading...'}
        descriptionNumberOfLines={1}
        right={() => (
          <Button
            compact
            onPress={() => copyToClipboard('PQC Key', pqcPubkey)}>
            {copied === 'PQC Key' ? 'Copied!' : 'Copy'}
          </Button>
        )}
      />

      <Divider />

      {addrs.length > 0 && (
        <>
          <Text variant="titleMedium" style={styles.sectionHeader}>
            Listen Addresses
          </Text>
          {addrs.map((addr, i) => (
            <List.Item
              key={i}
              title={addr}
              titleNumberOfLines={3}
              titleStyle={styles.addrText}
            />
          ))}
        </>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: {flex: 1},
  content: {padding: 20},
  heading: {marginBottom: 4},
  hint: {marginBottom: 20, opacity: 0.6},
  qrContainer: {alignItems: 'center', marginBottom: 20, padding: 20, backgroundColor: 'white', borderRadius: 12},
  qrCopyButton: {marginTop: 12},
  divider: {marginVertical: 4},
  sectionHeader: {marginTop: 24, marginBottom: 8},
  addrText: {fontSize: 12, fontFamily: 'monospace'},
});
