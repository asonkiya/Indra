import React, {useCallback, useEffect, useRef, useState} from 'react';
import {FlatList, KeyboardAvoidingView, Platform, StyleSheet, View} from 'react-native';
import {Text, TextInput, IconButton, useTheme} from 'react-native-paper';
import type {NativeStackScreenProps} from '@react-navigation/native-stack';
import type {RootStackParamList} from '../../App';
import indra, {type InboundMessage} from '../native/IndraModule';

type Props = NativeStackScreenProps<RootStackParamList, 'Chat'>;

export default function Chat({route}: Props) {
  const {conversationId, isGroup, participants} = route.params;
  const [messages, setMessages] = useState<InboundMessage[]>([]);
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const flatListRef = useRef<FlatList>(null);
  const theme = useTheme();

  const loadMessages = useCallback(async () => {
    try {
      const msgs = await indra.getMessages(conversationId, 200);
      setMessages(msgs);
    } catch {
      // not ready
    }
  }, [conversationId]);

  useEffect(() => {
    loadMessages();
  }, [loadMessages]);

  // Listen for new inbound messages in this conversation.
  useEffect(() => {
    const unsub = indra.onMessage((msg: InboundMessage) => {
      if (msg.conversation_id === conversationId) {
        setMessages(prev => [...prev, msg]);
      }
    });
    return unsub;
  }, [conversationId]);

  const handleSend = async () => {
    const trimmed = text.trim();
    if (!trimmed || sending) {
      return;
    }

    setSending(true);
    try {
      if (isGroup) {
        await indra.sendGroupMessage(conversationId, trimmed);
      } else {
        // Find the other participant (not us).
        const myPeerID = await indra.peerID();
        const recipient = participants.find(p => p !== myPeerID) || participants[0];
        await indra.sendMessage(recipient, trimmed);
      }

      // Optimistically add the sent message.
      const sentMsg: InboundMessage = {
        id: Date.now().toString(),
        conversation_id: conversationId,
        sender_id: await indra.peerID(),
        text: trimmed,
        sent_at_unix: Math.floor(Date.now() / 1000),
        direction: 'outbound',
      };
      setMessages(prev => [...prev, sentMsg]);
      setText('');
    } catch {
      // TODO: show error toast
    } finally {
      setSending(false);
    }
  };

  const renderMessage = ({item}: {item: InboundMessage}) => {
    const isOutbound = item.direction === 'outbound';
    return (
      <View
        style={[
          styles.bubble,
          isOutbound ? styles.outbound : styles.inbound,
          {
            backgroundColor: isOutbound
              ? theme.colors.primaryContainer
              : theme.colors.surfaceVariant,
          },
        ]}>
        {isGroup && !isOutbound && (
          <Text variant="labelSmall" style={styles.senderLabel}>
            {item.sender_id.slice(0, 12)}...
          </Text>
        )}
        <Text variant="bodyMedium">{item.text}</Text>
        <Text variant="labelSmall" style={styles.timestamp}>
          {new Date(item.sent_at_unix * 1000).toLocaleTimeString([], {
            hour: '2-digit',
            minute: '2-digit',
          })}
        </Text>
      </View>
    );
  };

  return (
    <KeyboardAvoidingView
      style={[styles.container, {backgroundColor: theme.colors.background}]}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      keyboardVerticalOffset={90}>
      <FlatList
        ref={flatListRef}
        data={messages}
        keyExtractor={item => item.id}
        renderItem={renderMessage}
        contentContainerStyle={styles.messageList}
        onContentSizeChange={() =>
          flatListRef.current?.scrollToEnd({animated: true})
        }
      />
      <View style={[styles.inputRow, {backgroundColor: theme.colors.surface}]}>
        <TextInput
          mode="outlined"
          placeholder="Message..."
          value={text}
          onChangeText={setText}
          onSubmitEditing={handleSend}
          style={styles.input}
          dense
        />
        <IconButton
          icon="send"
          mode="contained"
          onPress={handleSend}
          disabled={!text.trim() || sending}
        />
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {flex: 1},
  messageList: {padding: 12, paddingBottom: 8},
  bubble: {
    maxWidth: '80%',
    padding: 10,
    borderRadius: 12,
    marginVertical: 3,
  },
  outbound: {alignSelf: 'flex-end', borderBottomRightRadius: 4},
  inbound: {alignSelf: 'flex-start', borderBottomLeftRadius: 4},
  senderLabel: {opacity: 0.6, marginBottom: 2},
  timestamp: {opacity: 0.5, marginTop: 4, alignSelf: 'flex-end'},
  inputRow: {
    flexDirection: 'row',
    alignItems: 'center',
    padding: 8,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: '#ccc',
  },
  input: {flex: 1, marginRight: 4},
});
