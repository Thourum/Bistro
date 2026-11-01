# Design System Document: The Architectural Monolith

## 1. Overview & Creative North Star
**Creative North Star: The Technical Atelier**

This design system is a rejection of the "soft" web. It moves away from the organic, rounded trends of the modern era and returns to the precision of Swiss International Style and architectural blueprints. It is designed for high-performance environments where information density is a feature, not a bug. 

By utilizing a **Zero-Radius Architecture**, we create a visual language of absolute structural integrity. The "template" look is broken through a hyper-disciplined adherence to the grid, intentional asymmetry in data visualization, and the rhythmic interplay between expansive white space and dense technical clusters. This is not just "minimalism"—it is an aggressive focus on the essential, where every pixel must justify its existence.

---

## 2. Colors
The palette is a clinical study in light and function. We use tonal shifts to define space, moving away from decorative color toward purely navigational and informational utility.

### Surface Hierarchy & The "No-Line" Rule
While the original request suggested borders, we will elevate the experience by enforcing a **Tonal Boundary** strategy. 
- **The "No-Line" Rule:** Prohibit 1px solid borders for primary sectioning. Instead, define global regions through subtle shifts in surface tokens. 
- **Nesting:** Use `surface_container_lowest` (#ffffff) for the primary workspace, nested within a `surface` (#f8f9fa) viewport. For high-density sidebars or utility panels, transition to `surface_container_high` (#e7e8e9).

### Signature Textures & Gradients
To prevent the UI from feeling "dead," we introduce the **Technical Gradient**. 
- **Main CTAs:** Transition from `primary` (#004ac6) to `primary_container` (#2563eb) at a 45-degree angle. This adds a microscopic level of depth that suggests a "lit" screen rather than a flat sticker.
- **Glassmorphism:** For overlays or command palettes, use `surface` at 85% opacity with a `20px` backdrop blur. This maintains the light-mode ethos while providing a high-end, layered feel.

---

## 3. Typography
The typographic system utilizes a "Dual-Engine" approach: **Geist** for the human interface and **JetBrains Mono** for the machine data.

*   **Geist (The Interface):** Used for all `display`, `headline`, and `title` roles. It provides a neutral, high-legibility foundation that feels "engineered."
*   **JetBrains Mono (The Data):** Used for all `label` roles, numerical data, and technical metadata. This creates a clear cognitive bridge: if it’s in Mono, it is a variable or a value.

### Scale Strategy
- **Display-LG (3.5rem):** Set with tight tracking (-0.02em) for an editorial, "poster" impact on landing states.
- **Label-SM (0.6875rem):** Utilize JetBrains Mono in All-Caps for utility labels to maximize density without sacrificing clarity.

---

## 4. Elevation & Depth
In a system with zero border-radius and no traditional shadows, depth is a product of **Tonal Layering** and **Atmospheric Perspective**.

### The Layering Principle
Hierarchy is achieved by stacking surface-container tiers. 
- **Level 0 (Base):** `surface` (#f8f9fa)
- **Level 1 (Card/Section):** `surface_container_lowest` (#ffffff)
- **Level 2 (Active/Popup):** `surface_container_low` (#f3f4f5)

### Ambient Shadows & Ghost Borders
- **Floating Elements:** When a shadow is non-negotiable (e.g., a critical popover), use an **Ambient Shadow**: `0 10px 30px rgba(25, 28, 29, 0.04)`. It should feel like a soft glow of light rather than a dark edge.
- **The "Ghost Border" Fallback:** For maximum density in data tables where background shifts aren't enough, use a 1px line using `outline_variant` at **20% opacity**. It must be felt, not seen.

---

## 5. Components

### Buttons
- **Primary:** Square edges. Background: `primary` to `primary_container` gradient. Text: `on_primary` (Geist, Medium).
- **Secondary:** Square edges. Background: `transparent`. Border: 1px `outline` (#737686).
- **Tertiary:** JetBrains Mono, All-Caps, `primary` text. No container.

### Input Fields
- **State Architecture:** Default is a bottom-only border of 1px `outline_variant`. On focus, transition to a full 1px `primary` border.
- **Information Density:** Labels sit inside the field boundaries in `label-sm` (JetBrains Mono) to save vertical space.

### Cards & Lists
- **The Divider Ban:** Strictly forbid horizontal divider lines. Use **8px, 16px, or 24px** increments of white space to separate list items. 
- **Hover States:** Instead of a border or shadow, use a subtle shift to `surface_container_highest` (#e1e3e4) to indicate interactivity.

### Technical Data Grid (Additional Component)
- A high-density component using `surface_container_low` for headers and `surface_container_lowest` for rows. All numerical values must be JetBrains Mono, tabular-lined (monospaced numbers) to ensure columns align perfectly for rapid scanning.

---

## 6. Do’s and Don’ts

### Do
*   **Do** embrace the square corner. It is the signature of the system.
*   **Do** use JetBrains Mono for any value that changes (IDs, timestamps, prices).
*   **Do** use intentional asymmetry—align headers to the far left and metadata to the far right to create "tension" across the white space.
*   **Do** use `primary` color sparingly. It should be a beacon, not a wash.

### Don’t
*   **Don’t** use border-radius, even for icons or small tags. 0px is the rule.
*   **Don’t** use standard 100% opaque grey borders for sectioning. Use background shifts.
*   **Don’t** use large, dark drop shadows. They muddy the clinical aesthetic.
*   **Don’t** use icons without purpose. If a word is clearer, use the word in `label-md`.