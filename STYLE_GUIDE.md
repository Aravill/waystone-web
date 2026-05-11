# Waystone Web Style Guide

This document defines the visual and design conventions used across the Waystone Web application. All new features and pages should follow these guidelines to maintain a consistent, cohesive user experience.

## Design Philosophy

**Minimalistic, Terminal-Inspired Aesthetic**

- Dark, focused interface that puts content first
- Monospace font for a technical, precise feel
- Limited color palette emphasizing clarity over decoration
- Subtle borders and spacing for visual structure
- Responsive mobile-first approach

---

## Color Palette

### Primary Colors

| Purpose | Color | Hex | Usage |
|---------|-------|-----|-------|
| Background | Pure Black | `#000000` | Page background, default container background |
| Text (Primary) | Off-White | `#ffffff` | Headings, high-contrast text |
| Text (Secondary) | Light Gray | `#e0e0e0` | Body text, primary content |
| Text (Tertiary) | Medium Gray | `#888888` | Descriptions, metadata, secondary info |
| Text (Disabled) | Dark Gray | `#555555` | Disabled state text, placeholders |

### Accent Colors

| Purpose | Color | Hex | Usage |
|---------|-------|-----|-------|
| Accent / Interactive | Cyan | `#00d9ff` | Links, buttons, focus states, active borders |
| Success | Green | `#00ff7f` | Success messages, confirmations |
| Error | Hot Pink | `#ff006e` | Error messages, warnings, destructive actions |

### Semantic Colors

| Purpose | Background | Text | Border |
|---------|-----------|------|--------|
| Success Message | `#0a1a0a` | `#00ff7f` | `#00ff7f` |
| Error Message | `#1a0a0a` | `#ff006e` | `#ff006e` |
| Disabled State | `#0a0a0a` | `#555555` (opacity 0.5) | `#2a2a2a` |
| Active Element | `#0a1a1a` | `#ffffff` | `#00d9ff` |

### Border & Structure

| Element | Color | Hex |
|---------|-------|-----|
| Primary Borders | Dark Gray | `#333333` |
| Subtle Borders | Darker Gray | `#2a2a2a` |
| Dividers | Dark Gray | `#333333` |

---

## Typography

### Font Family

```css
font-family: 'Fira Code', monospace;
```

**Fira Code** is imported from Google Fonts. It's monospaced, professional, and evokes a technical terminal aesthetic.

### Font Sizes & Weights

#### Desktop

| Element | Size | Weight | Letter Spacing | Usage |
|---------|------|--------|-----------------|-------|
| Page Heading (h1) | 1.5em (24px) | 500 | 2px | Main page title |
| Section Heading (h2) | 1.2em (19px) | 400 | 1px | Section titles |
| Card Heading (h3) | 1.1em (18px) | 400 | 1px | Module/card titles |
| Body Text | 0.95em (15px) | 400 | normal | Form labels, descriptions |
| Small Text | 0.85em (14px) | 400 | normal | Metadata, secondary info |
| Button Text | 0.85em (14px) | 400 | 1px | Buttons, links |
| Badge Text | 0.75em (12px) | 400 | 0.5px | Status badges |

#### Mobile

Reduce by 1 step (e.g., h1: 1.2em → 24px becomes 20px). Maintain hierarchy and letter-spacing.

---

## Layout & Spacing

### Grid System

- **Container Max Width**: 900px (for modular layouts like dashboard)
- **Narrow Container Max Width**: 600px (for forms, like event signup)
- **Mobile Padding**: 20px (on smaller screens)
- **Desktop Padding**: 40px (standard horizontal padding)

### Spacing Scale

```
Base Unit: 5px

- 10px (2 units): Minimal spacing between related elements
- 15px (3 units): Intra-group spacing
- 20px (4 units): Small gaps, medium element spacing
- 25px (5 units): Medium gaps
- 30px (6 units): Large gaps, section spacing
- 40px (8 units): Extra-large spacing, page padding
```

### Responsive Breakpoints

```css
@media (max-width: 768px) {
    /* Mobile layout adjustments */
    /* Single-column grids, reduced padding */
}
```

---

## Components

### Buttons

#### Primary Button (CTA)

```css
.submit-btn, .logout-btn, .action-btn {
    padding: 12px 16px;
    background: #000000;
    color: #00d9ff;
    border: 1px solid #00d9ff;
    font-family: 'Fira Code', monospace;
    font-size: 0.85em;
    transition: all 0.3s;
    text-transform: uppercase;
    letter-spacing: 1px;
    cursor: pointer;
}

.submit-btn:hover {
    background: #00d9ff;
    color: #000000;
}

.submit-btn:active {
    opacity: 0.8;
}
```

**States:**
- Default: Transparent bg, cyan text & border
- Hover: Cyan bg, black text
- Active: Opacity 0.8
- Disabled: Grayed out (opacity 0.5), cursor: not-allowed

### Form Inputs

```css
input, select {
    width: 100%;
    padding: 12px;
    border: 1px solid #333333;
    background: #0a0a0a;
    color: #e0e0e0;
    font-family: 'Fira Code', monospace;
    font-size: 0.95em;
    transition: border-color 0.3s;
}

input::placeholder {
    color: #555555;
}

input:focus, select:focus {
    outline: none;
    border-color: #00d9ff;
}
```

**States:**
- Default: Dark bg, light gray text, subtle border
- Focus: Cyan border highlight
- Disabled: Grayed out, cursor: not-allowed

### Container / Card

```css
.container, .card {
    background: #0a0a0a;
    border: 1px solid #333333;
    padding: 30px; /* or appropriate value */
}

.container {
    max-width: 600px; /* narrow forms */
    background: #000000; /* for root containers */
}
```

**Usage:**
- `.container`: Wraps main content (e.g., signup form)
- `.card`: Modular content blocks (e.g., event items, module cards)

### Messages (Success / Error)

```css
.message {
    margin-top: 15px;
    padding: 12px;
    display: none;
    text-align: center;
    font-size: 0.9em;
    border: 1px solid;
}

.message.success {
    background: #0a1a0a;
    color: #00ff7f;
    border-color: #00ff7f;
    display: block;
}

.message.error {
    background: #1a0a0a;
    color: #ff006e;
    border-color: #ff006e;
    display: block;
}
```

**Auto-hide**: Messages should auto-hide after 5 seconds via JavaScript.

### Module Cards (Dashboard)

```css
.module-card {
    background: #0a0a0a;
    border: 1px solid #333333;
    padding: 30px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: all 0.3s;
    text-decoration: none;
    color: inherit;
    min-height: 180px;
}

.module-card.active {
    border-color: #00d9ff;
    background: #0a1a1a;
}

.module-card.active:hover {
    border-color: #00d9ff;
    box-shadow: 0 0 20px rgba(0, 217, 255, 0.3);
    transform: translateY(-2px);
}

.module-card.disabled {
    background: #0a0a0a;
    border-color: #2a2a2a;
    opacity: 0.5;
    cursor: not-allowed;
}
```

**States:**
- Active: Cyan border, slightly raised on hover with glow
- Disabled: Grayed out, no hover effects

### Status Badges

```css
.status-badge {
    display: inline-block;
    padding: 6px 12px;
    font-size: 0.75em;
    letter-spacing: 0.5px;
    background: #0a0a0a;
    border: 1px solid #00d9ff;
    color: #00d9ff;
    text-transform: uppercase;
}

.status-badge.coming-soon {
    background: #1a0a0a;
    border: 1px solid #ff006e;
    color: #ff006e;
}
```

### Campaign Status Badge

**Purpose:** Distinguish campaign lifecycle states (Pitch, Ongoing, Finished, On Hiatus, Cancelled) with visual badges below the campaign title.

**Visual Design:**
- Label-like appearance: no border, colored text with subtle background tint
- Uppercase text with letter spacing for clarity
- Responsive glow effect on hover

**Status Colors:**
- **Pitch** (#ffd700 Gold): Campaign pitch phase - recruiting players
- **Ongoing** (#00d9ff Cyan): Campaign is actively running
- **Finished** (#00ff7f Green): Campaign has concluded
- **On Hiatus** (#ff8c00 Orange): Campaign is temporarily paused
- **Cancelled** (#ff006e Pink): Campaign has been cancelled

**Hover Behavior:**
- Text glows with status-specific color
- Tooltip appears above badge on hover and keyboard focus
- Keyboard focus includes clear focus-visible outline

**Position:** Below the campaign title in the campaign card

**CSS Example:**
```css
.campaign-status-badge {
    display: inline-block;
    position: relative;
    padding: 4px 10px;
    border: none;
    font-size: 0.75em;
    font-weight: 600;
    letter-spacing: 0.5px;
    text-transform: uppercase;
    transition: text-shadow 0.2s ease;
}

/* Tooltip appears on hover and focus */
.campaign-status-badge::before {
    content: attr(data-tooltip);
    position: absolute;
    background: #0a0a0a;
    border: 1px solid;
    padding: 6px 10px;
    border-radius: 2px;
    font-size: 0.85em;
    white-space: nowrap;
    bottom: 120%;
    left: 50%;
    transform: translateX(-50%);
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.2s;
    z-index: 1000;
}

.campaign-status-badge:hover::before,
.campaign-status-badge:focus::before,
.campaign-status-badge:focus-visible::before {
    opacity: 1;
}

/* Clear focus-visible outline */
.campaign-status-badge:focus-visible {
    outline: 2px solid #00d9ff;
    outline-offset: 2px;
}

/* Status variants */
.campaign-item.status-pitch .campaign-status-badge {
    background: rgba(255, 215, 0, 0.1);
    color: #ffd700;
}

.campaign-item.status-ongoing .campaign-status-badge {
    background: rgba(0, 217, 255, 0.1);
    color: #00d9ff;
}

.campaign-item.status-finished .campaign-status-badge {
    background: rgba(0, 255, 127, 0.1);
    color: #00ff7f;
}

.campaign-item.status-hiatus .campaign-status-badge {
    background: rgba(255, 140, 0, 0.1);
    color: #ff8c00;
}

.campaign-item.status-cancelled .campaign-status-badge {
    background: rgba(255, 0, 110, 0.1);
    color: #ff006e;
}

/* Hover glow effects */
.campaign-item.status-pitch .campaign-status-badge:hover {
    text-shadow: 0 0 10px rgba(255, 215, 0, 0.6), 0 0 20px rgba(255, 215, 0, 0.3);
}

.campaign-item.status-ongoing .campaign-status-badge:hover {
    text-shadow: 0 0 10px rgba(0, 217, 255, 0.6), 0 0 20px rgba(0, 217, 255, 0.3);
}

.campaign-item.status-finished .campaign-status-badge:hover {
    text-shadow: 0 0 10px rgba(0, 255, 127, 0.6), 0 0 20px rgba(0, 255, 127, 0.3);
}

.campaign-item.status-hiatus .campaign-status-badge:hover {
    text-shadow: 0 0 10px rgba(255, 140, 0, 0.6), 0 0 20px rgba(255, 140, 0, 0.3);
}

.campaign-item.status-cancelled .campaign-status-badge:hover {
    text-shadow: 0 0 10px rgba(255, 0, 110, 0.6), 0 0 20px rgba(255, 0, 110, 0.3);
}
```

**Required HTML Attributes (generated in JS):**
- `data-tooltip`: Content displayed in tooltip pseudo-element
- `title`: Accessible tooltip text for assistive technologies and native browser tooltips
- `tabindex="0"`: Makes badge keyboard focusable
- `aria-label`: Semantic label combining status name and meaning for screen readers

**Do's:**
- Use for campaign lifecycle state indication
- Apply glow effect on hover
- Show tooltip on hover and keyboard focus with descriptive text
- Keep consistent with status colors across the campaign card

**Don'ts:**
- Don't use as a button or make it look clickable
- Don't add borders - use color for text and background tint only
- Don't add click handlers to the badge
- Don't change status colors for different contexts

---

## Interactions

### Transitions

All interactive elements use a **0.3s transition** for hover/focus states:

```css
transition: all 0.3s;
```

### Hover Effects

**Active Elements:**
- Border color shift (→ cyan)
- Background subtle change
- Optional: small lift (transform: translateY(-2px))
- Optional: subtle glow (box-shadow with rgba)

**Disabled Elements:**
- No hover effects
- cursor: not-allowed

### Focus States

All focusable elements (inputs, buttons, links) should have clear focus indication:

```css
outline: none; /* remove default */
border-color: #00d9ff; /* or similar accent */
```

---

## Grid Layouts

### Dashboard Module Grid

```css
.modules-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: 25px;
    width: 100%;
}
```

**Mobile**: Switches to single column (1fr) with reduced gap (20px).

### Event List

```css
.events-list {
    background: #0a0a0a;
    padding: 15px;
    max-height: 200px;
    overflow-y: auto;
    border: 1px solid #2a2a2a;
}

.event-item {
    padding: 10px;
    margin-bottom: 10px;
    background: #111111;
    border-left: 2px solid #00d9ff;
}
```

---

## Best Practices

### Do's

✅ Use Fira Code for all text
✅ Maintain consistent spacing (multiples of 5px)
✅ Keep color palette to the 8 primary colors + neutrals
✅ Use cyan (#00d9ff) for interactive/hover states
✅ Include letter-spacing in headings (1px-2px)
✅ Use transitions for smooth interactions (0.3s)
✅ Test on mobile and desktop
✅ Ensure sufficient color contrast (especially text)

### Don'ts

❌ Don't use gradients (except in special cases like previous gradient backgrounds)
❌ Don't add border-radius to cards (keep them sharp, minimal)
❌ Don't use drop shadows (exception: subtle glow on active elements)
❌ Don't introduce new colors outside the palette
❌ Don't animate text size or font-weight
❌ Don't use animations longer than 0.3s for routine interactions
❌ Don't add decorative elements that clutter the interface

---

## Code Examples

### New Page Template

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Page Title</title>
    <link rel="stylesheet" href="styles.css">
    <link rel="stylesheet" href="page-specific.css">
</head>
<body>
    <div class="header">
        <h1>Page Title</h1>
        <button class="logout-btn">Logout</button>
    </div>

    <div class="container">
        <h2>Section Title</h2>
        <form>
            <div class="form-group">
                <label for="input">Label</label>
                <input id="input" type="text" placeholder="Placeholder">
            </div>
            <button type="submit" class="submit-btn">Submit</button>
        </form>
        <div class="message success">Success message</div>
        <div class="message error">Error message</div>
    </div>

    <script src="page-specific.js"></script>
</body>
</html>
```

### CSS Template

```css
@import url('https://fonts.googleapis.com/css2?family=Fira+Code:wght@400;500;700&display=swap');

/* Reset */
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

/* Base Styles */
body {
    font-family: 'Fira Code', monospace;
    background: #000000;
    color: #e0e0e0;
    /* ... */
}

/* Components */
.header {
    /* ... */
}

/* Responsive */
@media (max-width: 768px) {
    /* Mobile styles */
}
```

---

## Profile Components

### User Button

```css
.user-button {
    display: inline-block;
    padding: 8px 12px;
    background: #0a1a1a;
    border: 1px solid #00d9ff;
    color: #00d9ff;
    text-decoration: none;
    font-family: 'Fira Code', monospace;
    font-size: 0.85em;
    border-radius: 3px;
    transition: all 0.3s;
    cursor: pointer;
}

.user-button:hover {
    background: #00d9ff;
    color: #000000;
}
```

**Usage**: Render user references as interactive buttons linking to user profiles. Always use `display_name` (computed: nickname → name → email).

### Profile Card

```css
.profile-card {
    background: #0a0a0a;
    border: 1px solid #333333;
    padding: 30px;
    margin-bottom: 30px;
}

.profile-avatar-section {
    display: flex;
    gap: 25px;
    align-items: flex-start;
}

.profile-avatar {
    width: 80px;
    height: 80px;
    border-radius: 5px;
    background: #1a1a1a;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    flex-shrink: 0;
}

.profile-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.profile-avatar-initials {
    font-size: 2em;
    font-weight: 500;
    color: #00d9ff;
}

.profile-display-name {
    font-size: 1.3em;
    color: #ffffff;
    margin-bottom: 10px;
}

.profile-meta {
    font-size: 0.9em;
    color: #b0b0b0;
    margin-bottom: 8px;
}
```

### Danger Zone

```css
.danger-zone {
    border: 1px solid #ff006e;
    background: #1a0a0a;
    padding: 25px;
    margin-top: 40px;
}

.danger-zone h2 {
    color: #ff006e;
    margin-bottom: 15px;
}

.danger-btn {
    padding: 12px 20px;
    background: transparent;
    border: 1px solid #ff006e;
    color: #ff006e;
    font-family: 'Fira Code', monospace;
    cursor: pointer;
    transition: all 0.3s;
    text-transform: uppercase;
    letter-spacing: 1px;
}

.danger-btn:hover {
    background: #ff006e;
    color: #000000;
}
```

### Campaign Item (Profile Context)

```css
.campaign-item-profile {
    background: #0a1a1a;
    border: 1px solid #333333;
    padding: 20px;
    margin-bottom: 15px;
    border-radius: 3px;
}

.campaign-item-profile h3 {
    margin-bottom: 10px;
    color: #ffffff;
}

.campaign-status {
    font-size: 0.9em;
    color: #b0b0b0;
    margin: 0;
}
```

---

## Maintenance

**Version**: 1.0  
**Last Updated**: 2024  
**Next Review**: When adding new major features or pages

### Unified Header & Navigation

All pages use a consistent two-part header system:

#### Header Structure (3-part layout)

```html
<div class="header">
    <div class="header-left">
        <a href="/" class="logo">Waystone</a>
    </div>
    <div class="header-center">
        <h1>Page Title</h1>
    </div>
    <div class="header-right">
        <span class="user-name" id="userName">User Email</span>
        <button id="logoutBtn" class="logout-btn">Logout</button>
    </div>
</div>
```

**Purpose**: 
- **Left**: Branding link back to dashboard
- **Center**: Current page title
- **Right**: Authenticated user info and logout action

#### Navigation Bar

```html
<div class="nav-bar">
    <a href="/" class="nav-item active" data-page="dashboard">Dashboard</a>
    <a href="/campaigns" class="nav-item" data-page="campaigns">Campaigns</a>
    <a href="/profile" class="nav-item" data-page="profile">Profile</a>
</div>
```

**Purpose**: Shows available modules (Dashboard, Campaigns, Profile) with active indicator on current page. Users can navigate between sections.

#### Header Styling

```css
.header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 20px 40px;
    border-bottom: 1px solid #333333;
    gap: 20px;
}

.header-left .logo {
    color: #ffffff;
    font-size: 1.3em;
    letter-spacing: 2px;
    text-decoration: none;
}

.header-center h1 {
    color: #ffffff;
    font-size: 1.2em;
    letter-spacing: 1px;
    margin: 0;
}

.header-right {
    display: flex;
    align-items: center;
    gap: 15px;
}

.user-name {
    color: #b0b0b0;
    font-size: 0.9em;
}
```

#### Navigation Bar Styling

```css
.nav-bar {
    display: flex;
    gap: 0;
    padding: 0 40px;
    border-bottom: 1px solid #333333;
    background: #0a0a0a;
}

.nav-item {
    padding: 15px 20px;
    color: #888888;
    text-decoration: none;
    border-bottom: 2px solid transparent;
    transition: all 0.3s;
    font-size: 0.9em;
    text-transform: uppercase;
    letter-spacing: 1px;
}

.nav-item:hover {
    color: #e0e0e0;
    border-bottom-color: #00d9ff;
}

.nav-item.active {
    color: #ffffff;
    border-bottom-color: #00d9ff;
}
```

#### Mobile Responsive Header

On screens ≤ 768px:
- Header switches to column layout (stacked vertically)
- Font sizes reduced by 1 step
- Navigation items show with reduced padding
- Maintains full functionality

### Cache-Busting Strategy

For development, all HTML files should reference CSS with query parameters to prevent browser caching:

```html
<!-- Use version query param to bypass browser cache -->
<link rel="stylesheet" href="dashboard.css?v=1">
<link rel="stylesheet" href="styles.css?v=1">
```

Increment the version number (`v=1` → `v=2`) when making CSS changes to ensure browsers fetch the latest stylesheet.

When updating this guide:
1. Document the change in a brief note
2. Update the version number
3. Ensure all existing pages follow the new guideline
4. Get team consensus on major style changes
