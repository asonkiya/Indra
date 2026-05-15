import React, {useState} from 'react';
import {ScrollView, StyleSheet, View} from 'react-native';
import {
  Button,
  SegmentedButtons,
  Text,
  TextInput,
  useTheme,
} from 'react-native-paper';
import type {NativeStackScreenProps} from '@react-navigation/native-stack';
import type {RootStackParamList} from '../../App';
import indra from '../native/IndraModule';

type Props = NativeStackScreenProps<RootStackParamList, 'AddContact'>;

export default function AddContact({navigation}: Props) {
  const [tab, setTab] = useState('qr');

  // QR / Paste JSON tab
  const [whoamiJson, setWhoamiJson] = useState('');

  // Manual tab
  const [peerID, setPeerID] = useState('');
  const [pubkeyHex, setPubkeyHex] = useState('');

  // Common
  const [alias, setAlias] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const theme = useTheme();

  const handleAddFromJson = async () => {
    setError('');
    const trimmed = whoamiJson.trim();
    if (!trimmed) {
      setError('Paste the Whoami JSON from your contact\'s Settings screen');
      return;
    }

    // Validate JSON parses before sending to native
    try {
      const parsed = JSON.parse(trimmed);
      if (!parsed.peer_id || !parsed.box_pubkey) {
        setError('Invalid Whoami JSON: missing peer_id or box_pubkey');
        return;
      }
    } catch {
      setError('Invalid JSON format');
      return;
    }

    setLoading(true);
    try {
      await indra.parseAndAddContact(trimmed, alias.trim());
      navigation.goBack();
    } catch (e: any) {
      setError(e.message || 'Failed to add contact');
    } finally {
      setLoading(false);
    }
  };

  const handleAddManual = async () => {
    setError('');
    const trimmedPeer = peerID.trim();
    const trimmedKey = pubkeyHex.trim();

    if (!trimmedPeer) {
      setError('Peer ID is required');
      return;
    }
    if (!trimmedKey || trimmedKey.length !== 64) {
      setError('Box public key must be 64 hex characters');
      return;
    }

    setLoading(true);
    try {
      await indra.addContact(trimmedPeer, trimmedKey, alias.trim());
      navigation.goBack();
    } catch (e: any) {
      setError(e.message || 'Failed to add contact');
    } finally {
      setLoading(false);
    }
  };

  return (
    <ScrollView
      style={[styles.container, {backgroundColor: theme.colors.background}]}
      contentContainerStyle={styles.content}
      keyboardShouldPersistTaps="handled">
      <SegmentedButtons
        value={tab}
        onValueChange={v => {
          setTab(v);
          setError('');
        }}
        buttons={[
          {value: 'qr', label: 'Paste QR / JSON'},
          {value: 'manual', label: 'Manual Entry'},
        ]}
        style={styles.tabs}
      />

      {tab === 'qr' ? (
        <>
          <Text variant="bodyMedium" style={styles.hint}>
            Ask your contact to open Settings, then copy their Whoami JSON and
            paste it below.
          </Text>

          <TextInput
            label="Whoami JSON"
            placeholder='{"peer_id":"12D3...","box_pubkey":"...","pqc_pubkey":"..."}'
            value={whoamiJson}
            onChangeText={setWhoamiJson}
            mode="outlined"
            autoCapitalize="none"
            autoCorrect={false}
            multiline
            numberOfLines={4}
            style={styles.input}
          />

          <TextInput
            label="Alias (optional)"
            placeholder="e.g. Alice"
            value={alias}
            onChangeText={setAlias}
            mode="outlined"
            style={styles.input}
          />

          {error ? (
            <Text
              variant="bodySmall"
              style={[styles.error, {color: theme.colors.error}]}>
              {error}
            </Text>
          ) : null}

          <View style={styles.buttons}>
            <Button
              mode="contained"
              onPress={handleAddFromJson}
              loading={loading}
              disabled={loading}>
              Add Contact
            </Button>
            <Button mode="outlined" onPress={() => navigation.goBack()}>
              Cancel
            </Button>
          </View>
        </>
      ) : (
        <>
          <Text variant="bodyMedium" style={styles.hint}>
            Enter your contact's Peer ID and Box public key manually.
          </Text>

          <TextInput
            label="Peer ID"
            placeholder="12D3KooW..."
            value={peerID}
            onChangeText={setPeerID}
            mode="outlined"
            autoCapitalize="none"
            autoCorrect={false}
            style={styles.input}
          />

          <TextInput
            label="Box Public Key (hex)"
            placeholder="64 hex characters"
            value={pubkeyHex}
            onChangeText={setPubkeyHex}
            mode="outlined"
            autoCapitalize="none"
            autoCorrect={false}
            style={styles.input}
          />

          <TextInput
            label="Alias (optional)"
            placeholder="e.g. Alice"
            value={alias}
            onChangeText={setAlias}
            mode="outlined"
            style={styles.input}
          />

          {error ? (
            <Text
              variant="bodySmall"
              style={[styles.error, {color: theme.colors.error}]}>
              {error}
            </Text>
          ) : null}

          <View style={styles.buttons}>
            <Button
              mode="contained"
              onPress={handleAddManual}
              loading={loading}
              disabled={loading}>
              Add Contact
            </Button>
            <Button mode="outlined" onPress={() => navigation.goBack()}>
              Cancel
            </Button>
          </View>
        </>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: {flex: 1},
  content: {padding: 20},
  tabs: {marginBottom: 20},
  hint: {marginBottom: 20, opacity: 0.7},
  input: {marginBottom: 16},
  error: {marginBottom: 12},
  buttons: {gap: 12, marginTop: 8},
});
