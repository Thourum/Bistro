import { defineConfig } from 'vocs/config'

export default defineConfig({
  title: 'Bistro',
  description: 'Documentation for Bistro, a keyboard-first GitHub PR review app.',
  renderStrategy: 'full-static',
  iconUrl: '/favicon.svg',
  editLink: {
    link: 'https://github.com/Itterno/Bistro/edit/master/website/src/pages/:path',
  },
  socials: [{ icon: 'github', link: 'https://github.com/Itterno/Bistro' }],
  sidebar: {
    '/docs': [
      {
        text: 'Welcome',
        link: '/docs',
      },
      {
        text: 'Keybindings',
        link: '/docs/keybindings',
      },
      {
        text: 'Local-first privacy',
        link: '/docs/privacy',
      },
      {
        text: 'Feedback',
        link: '/docs/feedback',
      },
    ],
  },
  topNav: [{ text: 'Docs', link: '/docs', match: '/docs' }],
})
