/**
 * Ant Design 5 theme — aligned with demo/button.html brand E (#1c6e8c)
 */
import type { ThemeConfig } from 'antd';
import { colors, rings } from './tokens';

export const antdTheme: ThemeConfig = {
  token: {
    colorPrimary: colors.brand,
    colorPrimaryHover: colors.brandInk,
    colorPrimaryActive: colors.brand700,
    colorPrimaryBg: colors.brandSoft,
    colorPrimaryBgHover: colors.brand100,
    colorPrimaryBorder: colors.brandLine,
    colorPrimaryBorderHover: colors.brand,
    colorPrimaryText: colors.brand,
    colorPrimaryTextHover: colors.brandInk,
    colorPrimaryTextActive: colors.brand700,

    colorSuccess: colors.green,
    colorSuccessBg: colors.greenSoft,
    colorSuccessBorder: colors.greenLine,
    colorSuccessText: colors.greenActive,

    colorError: colors.red,
    colorErrorBg: colors.redSoft,
    colorErrorBorder: colors.redLine,
    colorErrorText: colors.redActive,
    colorErrorHover: colors.redHover,
    colorErrorActive: colors.redActive,

    colorWarning: colors.amber,
    colorWarningBg: colors.amberSoft,
    colorWarningBorder: colors.amberLine,
    colorWarningText: colors.amberHover,

    colorInfo: colors.brand,
    colorInfoBg: colors.brandSoft,
    colorInfoBorder: colors.brandLine,
    colorInfoText: colors.brandInk,

    colorLink: colors.blue,
    colorLinkHover: '#2a45c8',
    colorLinkActive: '#243db0',

    colorText: colors.ink,
    colorTextSecondary: colors.ink2,
    colorTextTertiary: colors.ink3,
    colorTextQuaternary: colors.inkFaint,

    colorBorder: colors.line,
    colorBorderSecondary: colors.line2,
    colorBgContainer: colors.surface,
    colorBgElevated: colors.surface,
    colorBgLayout: colors.bg,
    colorBgSpotlight: colors.surface2,
    colorFillSecondary: colors.surface2,
    colorFillTertiary: colors.surface3,

    borderRadius: 6,
    borderRadiusLG: 8,
    borderRadiusSM: 5,
    fontFamily:
      "'Inter', system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
    controlOutline: rings.brand,
    controlOutlineWidth: 3,
  },
  components: {
    Button: {
      primaryShadow: `0 2px 0 ${rings.brand}`,
      defaultBorderColor: colors.line2,
      defaultColor: colors.ink,
      dangerColor: colors.red,
      borderRadius: 6,
      controlHeight: 30,
      controlHeightLG: 38,
      controlHeightSM: 28,
      paddingContentHorizontal: 13,
      fontWeight: 500,
    },
    Modal: {
      borderRadiusLG: 12,
      titleFontSize: 17,
      titleColor: colors.ink,
      contentBg: colors.surface,
      headerBg: colors.surface,
      footerBg: colors.surface,
      paddingContentHorizontalLG: 24,
      paddingMD: 20,
    },
    Popconfirm: {
      borderRadiusLG: 10,
    },
    Message: {
      contentBg: colors.surface,
    },
    Notification: {
      borderRadiusLG: 8,
    },
    Tag: {
      borderRadiusSM: 5,
    },
    Input: {
      activeBorderColor: colors.brand,
      hoverBorderColor: colors.brandLine,
      activeShadow: `0 0 0 3px ${rings.brand}`,
    },
    Select: {
      optionSelectedBg: colors.brandSoft,
    },
    Tabs: {
      inkBarColor: colors.brand,
      itemActiveColor: colors.brandInk,
      itemSelectedColor: colors.brandInk,
      itemHoverColor: colors.brand,
    },
    Switch: {
      colorPrimary: colors.green,
      colorPrimaryHover: colors.greenHover,
    },
  },
};

export default antdTheme;
