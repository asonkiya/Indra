import React, {useCallback, useEffect, useState} from 'react';
import {FlatList, StyleSheet, View} from 'react-native';
import {List, FAB, Text, useTheme} from 'react-native-paper';
import type {NativeStackScreenProps} from '@react-navigation/native-stack';
import type {RootStackParamList} from '../../App';
import indra, {type Conversation, type InboundMessage} from '../native/IndraModule';

type Props = NativeStackScreenProps<RootStackParamList, 'Conversations'>;

export default function ConversationList({navigation}: Props) {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const theme = useTheme();

  const load = useCallback(async () => {
    try {
      const convos = await indra.getConversations();
      setConversations(convos);
    } catch {
      // native module not ready yet
    }
  }, []);

  useEffect(() => {
    load();
    const unsubscribe = navigation.addListener('focus', load);
    return unsubscribe;
  }, [load, navigation]);

  // Refresh list when an inbound message arrives.
  useEffect(() => {
    const unsub = indra.onMessage((_msg: InboundMessage) => {
      load();
    });
    return unsub;
  }, [load]);

  const renderItem = ({item}: {item: Conversation}) => {
    const title = item.is_group
      ? `# ${item.name} [${item.participants.length}]`
      : item.name || item.id.slice(0, 16);

    return (
      <List.Item
        title={title}
        description={item.is_group ? 'Group chat' : 'Direct message'}
        left={props => (
          <List.Icon
            {...props}
            icon={item.is_group ? 'account-group' : 'account'}
          />
        )}
        onPress={() =>
          navigation.navigate('Chat', {
            conversationId: item.id,
            name: item.name,
            isGroup: item.is_group,
            participants: item.participants,
          })
        }
        style={styles.item}
      />
    );
  };

  return (
    <View style={[styles.container, {backgroundColor: theme.colors.background}]}>
      {conversations.length === 0 ? (
        <View style={styles.empty}>
          <Text variant="bodyLarge" style={styles.emptyText}>
            No conversations yet
          </Text>
          <Text variant="bodyMedium" style={styles.emptyHint}>
            Add a contact to start messaging
          </Text>
        </View>
      ) : (
        <FlatList
          data={conversations}
          keyExtractor={item => item.id}
          renderItem={renderItem}
        />
      )}
      <FAB
        icon="plus"
        style={styles.fab}
        onPress={() => navigation.navigate('AddContact')}
        label="Add Contact"
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: {flex: 1},
  item: {paddingHorizontal: 8},
  empty: {flex: 1, justifyContent: 'center', alignItems: 'center'},
  emptyText: {marginBottom: 8, opacity: 0.7},
  emptyHint: {opacity: 0.5},
  fab: {position: 'absolute', right: 16, bottom: 16},
});
