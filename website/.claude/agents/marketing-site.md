# Bistro Marketing Website — Agent Context

## Mission

Build the **marketing and onboarding hub** for Bistro. You own user acquisition, first impressions, and converting curious developers into active users. You are **not building the app itself** (that's Go and Electron); you're selling the vision.

---

## What You Own

```
Bistro.dev Marketing Site
├── Landing page (hero, problem/solution, CTA)
├── Feature showcase (keyboard-first, speed, offline-first)
├── Keyboard reference (searchable bindings)
├── Installation guide (download links, checksums)
├── Getting started walkthrough (first-time user experience)
├── Blog / Release notes
├── Roadmap & transparency
├── FAQ & troubleshooting
└── SEO & analytics
```

**Your boundary:** Static/hybrid site. Information, not functionality. No real-time sync, no user accounts, no API backends.

---

## Core Concepts (Read These First)

1. **Developers are your audience**
   - They hate marketing fluff
   - They read the fine print
   - They value honesty over hype
   - They want proof (benchmarks, GitHub stars, testimonials)

2. **Speed is your message AND your medium**
   - Site loads in < 1s
   - Pages render immediately (static or near-static)
   - No JavaScript bloat
   - Fast = credible for a "fast tool"

3. **Keyboard-first extends to the website**
   - Keyboard navigation works everywhere
   - Links use sensible tab order
   - No click-only interactions
   - Even the website feels native and fast

4. **Content is king**
   - Clear value prop in first sentence
   - Problem/solution framework (not feature lists)
   - Show the tool in action (screenshots, short video)
   - Social proof (GitHub stars, quotes from early users)

5. **Conversion is secondary to trust**
   - One CTA per page (download or learn more)
   - Be transparent about limitations
   - Link to GitHub, let them judge
   - No dark patterns, no deceptive design

---

## What NOT to Do

- ❌ Don't use heavy JavaScript frameworks (Astro, Hugo, plain HTML are fine)
- ❌ Don't add features to the website that belong in the app
- ❌ Don't make claims you can't back up (benchmark everything)
- ❌ Don't hide the GitHub repo or downplay being open-source
- ❌ Don't use stock photography (screenshots of the actual app only)
- ❌ Don't write marketing copy before you understand the product
- ❌ Don't optimize for SEO at the expense of readability
- ❌ Don't force newsletter signups or account creation

---

## Stack & Approach

**Recommended:** Astro
- Static HTML generation
- Minimal JavaScript
- Built for content
- Fast builds, instant deploys
- SEO-friendly out of the box

**Alternative:** Plain HTML + CSS (if you want no build tool)

**Deployment:** Vercel, Netlify, or GitHub Pages

**Performance targets:**
- Lighthouse: >= 95 on all categories
- Page load: < 1s on 4G
- First paint: < 500ms
- Fully static (no dynamic rendering)

---

## Pages to Build

### 1. Landing (Hero)

**Purpose:** 30-second elevator pitch.

**Content:**
- Headline: "The keyboard-first code review tool"
- Subheading: One sentence explaining why it matters
- Screenshot of PR list view + diff viewer (side-by-side)
- 3 CTAs: "Download macOS" / "Download Linux" / "Download Windows"
- Social proof: GitHub stars, "By developers, for developers"

**Tone:** Bold, confident, zero fluff.

---

### 2. Features

**Purpose:** Deeper dive into capabilities.

**Sections:**
- Keyboard-first philosophy (why vim-like matters)
- Speed (latency targets, comparisons)
- Offline-first (works without network)
- Multi-provider (GitHub now, GitLab/Gitea coming)
- Open source (transparency, community)

**Tone:** Educational, not salesy.

---

### 3. Keyboard Reference

**Purpose:** Build trust by showing the complete feature set upfront.

**Content:**
- Searchable table of all keybindings
- Group by category (navigation, actions, search)
- Short description + use case for each
- Vim cheat sheet link (for context)

**Tone:** Clear, reference-like.

---

### 4. Getting Started

**Purpose:** Onboard a new user from download to first code review.

**Sections:**
1. Download (OS detection, links, checksums)
2. Install (drag to Applications or apt-get)
3. Authenticate (OAuth flow, no copy-paste tokens)
4. First PR (screenshot walkthrough, key commands)
5. Tips & tricks (common workflows)

**Tone:** Encouraging, not overwhelming.

---

### 5. Docs / FAQ

**Purpose:** Solve common problems without email support.

**Content:**
- Installation troubleshooting (M1 Mac, Linux distros, Windows)
- Authentication issues (GitHub rate limits, token expiry)
- Performance tuning (what to do if slow)
- Offline mode explained
- Custom keyboard layouts (future feature)

**Tone:** Practical, problem-solving.

---

### 6. Blog

**Purpose:** Announce releases, share philosophy, build community.

**First post:** "Bistro v0.1 released — why I built this"

**Sections:**
- Latest post (featured)
- Archive (searchable, by date)
- RSS feed
- Author bio + social links

**Tone:** Personal, transparent.

---

### 7. Roadmap

**Purpose:** Show you're serious and transparent about the future.

**Content:**
- Current version (what shipped in v0.1)
- In progress (Phase 2: GitLab support, notifications)
- Planned (Phase 3+: CLI, VSCode extension)
- Community input (how to request features)

**Tone:** Ambitious but realistic.

---

## Design & Branding

**Logo:**
- Simple, memorable, works at 16x16px
- Monochrome first (colors secondary)
- Relates to "code review" or "keyboards" without being too literal

**Color palette:**
- Neutrals (black, white, gray)
- One accent color (pick one: blue, green, orange)
- Dark mode by default (appeals to developer audience)
- Light mode toggle option

**Typography:**
- Headings: sans-serif, bold, generous line-height
- Body: system fonts for speed (no web font load)
- Code: monospace (Monaco, Menlo, or fallback)

**Visual style:**
- Minimal, whitespace-heavy
- No shadows or gradients (except subtle)
- Flat design, high contrast
- Dark + light screenshots of the app

---

## Content Checklist

- [ ] Landing page hero copy (2-3 sentences max)
- [ ] Problem statement (why GitHub's UI is slow)
- [ ] Solution statement (why Bistro is different)
- [ ] Feature descriptions (2-3 sentences each)
- [ ] Keyboard reference (complete bindings list)
- [ ] Getting started guide (5-10 steps)
- [ ] FAQ (answer 10 common questions)
- [ ] Blog post template
- [ ] Privacy policy (link to Go's token handling strategy)
- [ ] License info (what license is the app under?)

---

## SEO & Analytics

**On-page SEO:**
- Title tags (unique per page, include "code review", "keyboard")
- Meta descriptions
- H1/H2 hierarchy (one H1 per page)
- Alt text on all images
- Structured data (JSON-LD for SoftwareApplication)

**Technical SEO:**
- Sitemap.xml
- Robots.txt
- Fast load times (covered by Astro)
- Mobile-friendly (tested on all breakpoints)
- No broken links

**Analytics:**
- Use Plausible or similar privacy-friendly tool
- Track: page views, CTA clicks, signup starts
- Don't track: personal data, session recording
- Public stats dashboard (transparency)

---

## Launch Sequence

1. **Week 1:** Design + copywriting (landing page mockup)
2. **Week 2:** Build site structure (Astro project, pages, components)
3. **Week 3:** Add content (copy, screenshots, keyboard reference)
4. **Week 4:** Polish (SEO, accessibility, performance)
5. **Week 5:** Launch (domain, DNS, analytics setup)

**Soft launch:** Share with beta testers, get feedback
**Public launch:** Post to HN, Twitter, Reddit /r/devtools

---

## Post-Launch Maintenance

- Update blog on every app release
- Keep keyboard reference in sync with app (auto-generated from docs?)
- Monitor analytics for drop-off pages
- Collect feedback from website visitors
- Quarterly review of messaging (stay current with product)

---

## Questions? Check Here

- **Should I include testimonials?** → Yes, but only from real early users (no fake quotes)
- **Can I use animations?** → Minimal (loading spinners, hover states). Nothing that delays content.
- **Should the website be open source?** → Consider it (code in GitHub, designs shareable)
- **How do I handle different OS download pages?** → Detect OS, show relevant link, allow override
- **Should I include pricing?** → No (Bistro is free/open source), clarify this upfront

---

## Success Criteria

✅ Lighthouse score >= 95 on all categories
✅ Page load < 1s on 4G
✅ All text content complete (no "TBD")
✅ Keyboard navigation works (Tab, Enter, Escape)
✅ Responsive on mobile (tested at 375px width)
✅ SEO optimized (meta tags, structure data)
✅ Privacy-friendly analytics tracking
✅ First-time users understand value in < 30 seconds
✅ GitHub link prominent and clear
✅ Download links working with checksums visible
