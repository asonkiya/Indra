import React, {useEffect, useState} from 'react';
import {StatusBar, useColorScheme} from 'react-native';
import {SafeAreaProvider} from 'react-native-safe-area-context';
import {NavigationContainer} from '@react-navigation/native';
import {createNativeStackNavigator} from '@react-navigation/native-stack';
import {
  MD3DarkTheme,
  MD3LightTheme,
  PaperProvider,
  ActivityIndicator,
} from 'react-native-paper';
import {View, StyleSheet, Text} from 'react-native';

import ConversationList from './src/screens/ConversationList';
import Chat from './src/screens/Chat';
import AddContact from './src/screens/AddContact';
import Settings from './src/screens/Settings';
import indra from './src/native/IndraModule';

export type RootStackParamList = {
  Conversations: undefined;
  Chat: {
    conversationId: string;
    name: string;
    isGroup: boolean;
    participants: string[];
  };
  AddContact: undefined;
  Settings: undefined;
};

const Stack = createNativeStackNavigator<RootStackParamList>();

function App() {
  const colorScheme = useColorScheme();
  const isDark = colorScheme === 'dark';
  const theme = isDark ? MD3DarkTheme : MD3LightTheme;

  const [ready, setReady] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    (async () => {
      try {
        // Pass empty string — native side resolves to the app's Documents dir,
        // which is sandboxed per simulator/device.
        await indra.start('');
        setReady(true);
      } catch (e: any) {
        setError(e.message || 'Failed to start Indra node');
      }
    })();

    return () => {
      indra.stop();
    };
  }, []);

  if (error) {
    return (
      <PaperProvider theme={theme}>
        <View style={styles.center}>
          <Text style={styles.error}>Error: {error}</Text>
        </View>
      </PaperProvider>
    );
  }

  if (!ready) {
    return (
      <PaperProvider theme={theme}>
        <View style={styles.center}>
          <ActivityIndicator size="large" />
          <Text style={styles.loading}>Starting Indra node...</Text>
        </View>
      </PaperProvider>
    );
  }

  return (
    <PaperProvider theme={theme}>
      <SafeAreaProvider>
        <StatusBar barStyle={isDark ? 'light-content' : 'dark-content'} />
        <NavigationContainer>
          <Stack.Navigator
            initialRouteName="Conversations"
            screenOptions={{
              headerStyle: {backgroundColor: theme.colors.surface},
              headerTintColor: theme.colors.onSurface,
            }}>
            <Stack.Screen
              name="Conversations"
              component={ConversationList}
              options={({navigation}) => ({
                title: 'Indra',
                headerRight: () => (
                  <Text
                    onPress={() => navigation.navigate('Settings')}
                    style={[styles.headerButton, {color: theme.colors.primary}]}>
                    Settings
                  </Text>
                ),
              })}
            />
            <Stack.Screen
              name="Chat"
              component={Chat}
              options={({route}) => ({
                title: route.params.name || 'Chat',
              })}
            />
            <Stack.Screen
              name="AddContact"
              component={AddContact}
              options={{title: 'Add Contact'}}
            />
            <Stack.Screen
              name="Settings"
              component={Settings}
              options={{title: 'Settings'}}
            />
          </Stack.Navigator>
        </NavigationContainer>
      </SafeAreaProvider>
    </PaperProvider>
  );
}

const styles = StyleSheet.create({
  center: {flex: 1, justifyContent: 'center', alignItems: 'center'},
  loading: {marginTop: 16, opacity: 0.6},
  error: {color: 'red', fontSize: 16, padding: 20, textAlign: 'center'},
  headerButton: {fontSize: 16, marginRight: 8},
});

export default App;
