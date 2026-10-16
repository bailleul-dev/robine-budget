import { useState } from 'react';
import {
  Drawer,
  DrawerContentScrollView,
  DrawerItem,
  DrawerItemList,
  type DrawerContentComponentProps,
} from 'expo-router/drawer';
import { Tabs } from 'expo-router/tabs';
import { Pressable, StyleSheet, View } from 'react-native';
import { Text } from '@/components/ui/text';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useAuth } from '@/auth/AuthContext';
import { useAppUpdate } from '@/components/AppUpdate';
import { TabBarBackground } from '@/components/TabBarBackground';
import { NavIcon, NavIcons } from '@/components/nav-icon';
import { Logo, LogoMark } from '@/components/ui/logo';
import { DrawerWidth, RailWidth, Spacing } from '@/constants/theme';
import { useIsDesktop } from '@/hooks/use-is-desktop';
import { useTheme } from '@/hooks/use-theme';

// The floating tab bar's own geometry — a "pill" inset from the bar's full
// (flow-reserved) width/height by these margins, rather than a bar flush
// with the screen edges. Kept in normal document flow (not position:
// 'absolute') specifically so react-navigation's own content-avoidance and
// safe-area math keep working unmodified — every screen already assumes
// the tab bar reserves real space below it.
const TAB_BAR_PILL_HEIGHT = 60;
const TAB_BAR_SIDE_MARGIN = 16;
const TAB_BAR_TOP_MARGIN = 8;
const TAB_BAR_BOTTOM_MARGIN = 12;
// On devices with a home indicator (insets.bottom > 0), the pill floats
// *into* the safe area rather than on top of it — stacking the full inset
// plus the margin left it hovering way too high on iOS. The indicator is a
// thin line at the very bottom, so the pill only needs to clear that.
const TAB_BAR_HOME_INDICATOR_CLEARANCE = 22;
// Fully rounded: the pill's own height, so its ends are half-circles.
const TAB_BAR_RADIUS = TAB_BAR_PILL_HEIGHT / 2;

type DrawerContentProps = DrawerContentComponentProps & {
  isDesktop: boolean;
  collapsed: boolean;
  onToggleCollapsed: () => void;
};

// The visible top-level screens every nav surface shares — Drawer.Screen
// (desktop sidebar), Tabs.Screen (mobile web/Android bottom bar, see
// MobileTabs below) each map this to their own options shape. Each one is a folder with its own Stack
// (components/TabStack.tsx), which draws the header: the navigators here
// never do. "(home)" also holds an account's detail
// (`/accounts/[accountId]`, pushed from its account list), so it stays
// inside the Comptes tab. "settings" is deliberately not in this list: the
// desktop drawer renders it from DrawerContent's footer instead (see
// below), while on mobile it's just the last tab.
const PRIMARY_SCREENS = [
  { name: '(home)', title: 'Comptes', icon: NavIcons.accounts },
  { name: 'graphs', title: 'Graphiques', icon: NavIcons.graphs },
  { name: 'planned-items', title: 'Budgets', icon: NavIcons.plannedItems },
  { name: 'categories', title: 'Catégories', icon: NavIcons.categories },
] as const;

/** Custom drawer content: a collapse toggle up top (desktop/permanent mode
 * only — the mobile front drawer is already dismissible by closing it), the
 * usual nav items (icons ported to every screen precisely so a collapsed,
 * icons-only rail still means something), then a footer pinned to the
 * bottom holding the signed-in user's email and, below it, "Réglages" —
 * its own DrawerItem, kept out of the main list (see the `settings`
 * screen's `drawerItemStyle` below) since it's "about this session" rather
 * than somewhere to navigate day-to-day. Sign-out itself lives on the
 * Réglages screen now, not here. */
function DrawerContent({ isDesktop, collapsed, onToggleCollapsed, ...props }: DrawerContentProps) {
  const theme = useTheme();
  const { user } = useAuth();
  const activeRouteName = props.state.routes[props.state.index]?.name;

  return (
    <View style={styles.drawerContainer}>
      {isDesktop && (
        <View style={[styles.toggleRow, { borderBottomColor: theme.line, justifyContent: collapsed ? 'center' : 'space-between', flexDirection: collapsed ? 'column' : 'row', gap: Spacing.two, borderBottomWidth: 0 }]}>
          {collapsed ? <LogoMark size={28} /> : <Logo size={30} />}
          <Pressable onPress={onToggleCollapsed} hitSlop={8} style={styles.toggleButton}>
            <NavIcon
              name={collapsed ? NavIcons.expandDrawer : NavIcons.collapseDrawer}
              color={theme.textSecondary}
              size={18}
            />
          </Pressable>
        </View>
      )}
      <DrawerContentScrollView {...props} contentContainerStyle={{ paddingTop: Spacing.two }}>
        <DrawerItemList {...props} />
      </DrawerContentScrollView>
      <View style={[styles.footer, { borderTopColor: theme.line }]}>
        {!collapsed && user?.email && (
          <Text style={[styles.email, { color: theme.textSecondary }]} numberOfLines={1}>
            {user.email}
          </Text>
        )}
        <DrawerItem
          label={collapsed ? () => null : 'Réglages'}
          icon={({ color, size }) => <NavIcon name={NavIcons.settings} color={String(color)} size={size} />}
          focused={activeRouteName === 'settings'}
          activeTintColor={theme.accentDark}
          activeBackgroundColor={theme.accentPale}
          inactiveTintColor={theme.textSecondary}
          labelStyle={styles.settingsLabel}
          style={styles.settingsItem}
          onPress={() => props.navigation.navigate('settings')}
        />
      </View>
    </View>
  );
}

/** Desktop nav: permanent (always-visible) sidebar. Can additionally be
 * collapsed to an icons-only rail (see `collapsed` state, toggled from
 * DrawerContent's top button). */
function DesktopDrawer() {
  const theme = useTheme();
  const [collapsed, setCollapsed] = useState(false);
  const hideLabel = collapsed ? (() => null) : undefined;

  return (
    <Drawer
      drawerContent={(props) => (
        <DrawerContent {...props} isDesktop collapsed={collapsed} onToggleCollapsed={() => setCollapsed((c) => !c)} />
      )}
      screenOptions={{
        headerShown: false,
        drawerType: 'permanent',
        drawerStyle: {
          width: collapsed ? RailWidth : DrawerWidth,
          backgroundColor: theme.surface,
          borderRightWidth: 0,
        },
        drawerItemStyle: { borderRadius: 16 },
        drawerActiveTintColor: theme.accentDark,
        drawerActiveBackgroundColor: theme.accentPale,
        drawerInactiveTintColor: theme.textSecondary,
        drawerLabelStyle: { fontFamily: 'Manrope_700Bold', fontSize: 14.5 },
      }}
    >
      {PRIMARY_SCREENS.map((s) => (
        <Drawer.Screen
          key={s.name}
          name={s.name}
          options={{
            title: s.title,
            drawerLabel: hideLabel ?? s.title,
            drawerIcon: ({ color, size }) => <NavIcon name={s.icon} color={String(color)} size={size} />,
          }}
        />
      ))}
      <Drawer.Screen
        name="settings"
        options={{ title: 'Réglages', drawerLabel: 'Réglages', drawerItemStyle: { display: 'none' } }}
      />
    </Drawer>
  );
}

/** Mobile tab order: Comptes, the home screen, comes first. */
const MOBILE_TAB_ORDER = ['(home)', 'graphs', 'planned-items', 'categories'] as const;
const HOME_TAB = '(home)';

export const unstable_settings = { initialRouteName: HOME_TAB };

/** Mobile nav (iOS, Android, mobile web): a floating bottom tab bar — the
 * usual phone pattern, styled as a pill inset from the screen edges rather
 * than a bar flush against them (see components/TabBarBackground.tsx, and
 * the TAB_BAR_* constants above for its geometry). Every visible screen
 * gets a tab, Réglages included. */
function MobileTabs() {
  const theme = useTheme();
  // A downloaded update waiting for a restart puts a dot on Réglages, where
  // the "Redémarrer" banner lives (see components/AppUpdate.tsx).
  const { isPending: updatePending } = useAppUpdate();
  const insets = useSafeAreaInsets();
  const bottomOffset = insets.bottom > 0 ? TAB_BAR_HOME_INDICATOR_CLEARANCE : TAB_BAR_BOTTOM_MARGIN;

  return (
    <Tabs
      initialRouteName={HOME_TAB}
      screenOptions={{
        headerShown: false,
        // position:'absolute' floats the bar over the screen's own content
        // (so it's real content, not empty background, showing through the
        // glass) instead of reserving flow space below it — every mobile
        // screen compensates with matching bottom padding via
        // useTabBarClearance(), fed by this same height through
        // react-navigation's own BottomTabBarHeightContext.
        tabBarStyle: {
          position: 'absolute',
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: 'transparent',
          borderTopWidth: 0,
          elevation: 0,
          height: TAB_BAR_PILL_HEIGHT + TAB_BAR_TOP_MARGIN + bottomOffset,
          paddingTop: TAB_BAR_TOP_MARGIN,
          paddingBottom: bottomOffset,
          paddingHorizontal: TAB_BAR_SIDE_MARGIN,
        },
        tabBarBackground: () => (
          <TabBarBackground
            top={TAB_BAR_TOP_MARGIN}
            bottom={bottomOffset}
            side={TAB_BAR_SIDE_MARGIN}
            radius={TAB_BAR_RADIUS}
          />
        ),
        tabBarActiveTintColor: theme.accentDark,
        tabBarInactiveTintColor: theme.textSecondary,
        tabBarLabelStyle: { fontFamily: 'Manrope_700Bold', fontSize: 10.5 },
      }}
    >
      {MOBILE_TAB_ORDER.map((name) => {
        const s = PRIMARY_SCREENS.find((screen) => screen.name === name)!;
        return (
          <Tabs.Screen
            key={s.name}
            name={s.name}
            options={{
              title: s.title,
              tabBarLabel: s.title,
              tabBarIcon: ({ color, size }) => <NavIcon name={s.icon} color={String(color)} size={size} />,
            }}
          />
        );
      })}
      <Tabs.Screen
        name="settings"
        options={{
          title: 'Réglages',
          tabBarLabel: 'Réglages',
          tabBarBadge: updatePending ? '' : undefined,
          tabBarBadgeStyle: { backgroundColor: theme.accent, minWidth: 10, maxHeight: 10, borderRadius: 5, top: 2 },
          tabBarIcon: ({ color, size }) => <NavIcon name={NavIcons.settings} color={String(color)} size={size} />,
        }}
      />
    </Tabs>
  );
}

export default function AppLayout() {
  const isDesktop = useIsDesktop();
  return isDesktop ? <DesktopDrawer /> : <MobileTabs />;
}

const styles = StyleSheet.create({
  drawerContainer: { flex: 1 },
  toggleRow: { flexDirection: 'row', alignItems: 'center', paddingHorizontal: Spacing.three, paddingVertical: Spacing.three },
  toggleButton: { padding: Spacing.one },
  footer: { borderTopWidth: 0, padding: Spacing.two },
  email: { fontSize: 12.5, paddingHorizontal: Spacing.two, paddingTop: Spacing.two, paddingBottom: Spacing.one },
  settingsItem: { borderRadius: 16 },
  settingsLabel: { fontFamily: 'Manrope_700Bold', fontSize: 14.5 },
});
