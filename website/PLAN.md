# Bistro Marketing Website — Plan

## Overview

A modern, fast landing page that communicates Bistro's value proposition to developers. The site is a static/hybrid build (Astro, Hugo, or Next.js static export) optimized for SEO, performance, and conversion.

**Stack:** TBD (Astro recommended for performance + content focus)

---

## Core Pages

### 1. **Home / Landing**

**Purpose:** Immediate value prop, screenshot, CTA to download.

**Sections:**
- Hero: "The keyboard-first code review tool" + screenshot of UI
- Problem: "Code review is slow. GitHub's web UI isn't made for speed."
- Solution: "Bistro: 10x faster, keyboard-native, open source"
- Features grid: Keyboard-first, blazingly fast, offline-first, cross-platform
- Screenshot carousel: showing key workflows (list view, diff, approve)
- CTA buttons: "Download for macOS" / "Download for Linux" / "Download for Windows"
- Social proof: GitHub stars, Twitter links, early adopter quotes

---

### 2. **Features**

**Purpose:** Deep dive into capabilities and philosophy.

**Sections:**
- Keyboard-first philosophy
  - Every action has a binding
  - Vim-like navigation
  - No mouse required
- Speed
  - Latency targets (startup, open PR, approve)
  - Local-first caching
  - Background sync
- Offline-first
  - Works without network
  - Seamless resync
- Multi-provider (roadmap)
  - GitHub first
  - GitLab / Gitea coming
- Open source
  - GitHub repo link
  - License info
  - Contributing guide

---

### 3. **Keyboard Reference**

**Purpose:** Searchable keybinding documentation for users.

**Sections:**
- Navigation (j/k, gg/G, etc.)
- Actions (a, c, r, etc.)
- Search & filtering
- Diff navigation
- Each binding has short description + use case

---

### 4. **Docs / Getting Started**

**Purpose:** Installation and first-run guide.

**Sections:**
- Installation: download links + checksums
- First run: OAuth setup, connecting GitHub
- First code review: step-by-step walkthrough
- Troubleshooting: common issues
- Settings / customization (if applicable)

---

### 5. **Blog / Updates**

**Purpose:** Announce releases, share philosophy, engage community.

**Sections:**
- Latest post: "Bistro v0.1 released"
- Archive: older posts
- RSS feed

---

### 6. **Roadmap**

**Purpose:** Transparency about upcoming work.

**Sections:**
- Current phase: what's shipping now
- Next phase: what's being built
- Future: long-term vision
- Community input: how to request features

---

## Visual & Content Strategy

### Design Philosophy
- **Minimal**: Clean, whitespace-heavy, focus on content
- **Fast**: Static HTML, no JavaScript frameworks on page load, instant
- **Developer-focused**: Code snippets, command-line friendly, no marketing fluff
- **Dark-mode by default** (option to switch): appeals to developer audience

### Key Messaging
- **Speed**: Every metric, every decision optimized for latency
- **Keyboard native**: "Never touch your mouse during a code review"
- **Local-first**: "Your data lives on your machine, not a server"
- **Open source**: "Transparent, community-driven"

### Social & Branding
- Logo: simple, memorable, works at small sizes
- Color palette: 2-3 colors, high contrast
- Typography: readable, monospace for code samples
- Imagery: app screenshots + minimalist illustrations (if any)

---

## Technical Requirements

### Performance Targets
- Lighthouse score: >= 95 on all categories
- Page load: < 1s on 4G
- Fully static HTML (no dynamic rendering needed)
- CDN-friendly: cacheable headers, gzip'd

### SEO & Analytics
- Meta tags: title, description, OG image for each page
- Structured data: JSON-LD for software application
- Analytics: plausible or similar privacy-friendly option
- Sitemap + RSS feed

### Content & Localization
- English first, Spanish/German/French later (if community demand)
- RTL-safe markup (for future Arabic/Hebrew)

---

## Build & Deployment

- **Build tool**: Astro (static gen, minimal JS, fast)
- **Hosting**: Vercel, Netlify, or GitHub Pages (free, automatic deploys)
- **Domain**: bistro.dev or similar
- **CI/CD**: GitHub Actions → build + deploy on push to main

---

## Asset Checklist

- [ ] Logo (SVG)
- [ ] Color palette / CSS variables
- [ ] Font stack (system fonts for speed, or Inter if needed)
- [ ] Screenshots of app (different themes: light/dark)
- [ ] Feature icons (6-8 icons for feature grid)
- [ ] Social media preview images (1200x630px)
- [ ] Favicon + app icons (iOS, Android, Windows)

---

## Content Checklist

- [ ] Hero copy (value prop in 2 sentences)
- [ ] Problem statement (pain points of current tools)
- [ ] Feature descriptions (2-3 sentences each)
- [ ] Keyboard reference (auto-generated from app docs?)
- [ ] Getting started guide (5-10 steps)
- [ ] FAQ (common questions)
- [ ] Blog post template

---

## Launch Sequence

1. **Week 1:** Design + copy (landing page mockup)
2. **Week 2:** Build site structure (Astro project, pages, components)
3. **Week 3:** Add content (copy, screenshots, docs)
4. **Week 4:** Polish (SEO, accessibility, performance tuning)
5. **Week 5:** Launch (domain, DNS, analytics setup)

**Soft launch:** Beta testers get link, collect feedback
**Public launch:** Post to HN, Twitter, Reddit /r/devtools

---

## Maintenance

- Update release notes / blog on each app version
- Keep keyboard reference in sync with app (auto-generated docs?)
- Monitor analytics for drop-off pages
- Collect user feedback from website CTAs
