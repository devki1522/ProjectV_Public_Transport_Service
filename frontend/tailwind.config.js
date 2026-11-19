// Token parity for Tailwind consumers; the current application renders with plain CSS.
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  theme: {
    extend: {
      fontFamily: { sans: ['var(--font-ui)'], mono: ['var(--font-numeric)'] },
      colors: {
        canvas: 'var(--bg-canvas)', surface: 'var(--bg-surface)', elevated: 'var(--bg-elevated)',
        primary: 'var(--text-primary)', secondary: 'var(--text-secondary)',
        hairline: 'var(--border-hairline)', action: 'var(--action)',
      },
      borderRadius: { DEFAULT: 'var(--radius-control)', panel: 'var(--radius-panel)', '2xl': 'var(--radius-panel)' },
      boxShadow: { md: 'none', lg: 'none' },
      fontSize: { eta: ['var(--eta-size)', { lineHeight: '1', fontWeight: '700' }] },
    },
  },
}
