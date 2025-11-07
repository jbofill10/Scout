## ADDED Requirements

### Requirement: Material-UI Custom Theme

The UI SHALL provide a centralized Material-UI theme configuration with warm, dark colors and design tokens.

#### Scenario: Define warm dark color palette

- **WHEN** the theme is created
- **THEN** the primary.main color SHALL be #4F46E5 (deep indigo)
- **AND** the primary.dark SHALL be #4338CA (darker indigo)
- **AND** the primary.light SHALL be #6366F1 (lighter indigo)
- **AND** the secondary.main SHALL be #8B5CF6 (warm purple)
- **AND** the secondary.dark SHALL be #7C3AED (darker purple)
- **AND** the secondary.light SHALL be #A78BFA (lighter purple)
- **AND** the background.default SHALL be #0F172A (very dark blue)
- **AND** the background.paper SHALL be #1E293B (dark indigo for cards)
- **AND** the text.primary SHALL be #F1F5F9 (off-white)
- **AND** the text.secondary SHALL be #94A3B8 (light gray-blue)

#### Scenario: Configure typography scale

- **WHEN** the theme is created
- **THEN** the font family SHALL be 'Inter', system-ui, sans-serif
- **AND** caption size SHALL be 12px
- **AND** body1 size SHALL be 14px
- **AND** h6 size SHALL be 16px
- **AND** h4 size SHALL be 24px
- **AND** font weights SHALL include 400 (regular), 500 (medium), 700 (bold)

#### Scenario: Override Material-UI component styles

- **WHEN** the theme is created
- **THEN** Card component SHALL have elevation 2 by default
- **AND** Button component SHALL use primary color for contained variant
- **AND** AppBar component SHALL use background color #0F172A
- **AND** Dialog component SHALL use paper background #1E293B

#### Scenario: Export theme for application

- **WHEN** the theme is configured
- **THEN** the theme SHALL be exported using createTheme()
- **AND** be TypeScript-typed for Material-UI v7
- **AND** be imported in App.tsx for ThemeProvider

### Requirement: Theme Provider Integration

The application SHALL integrate the custom theme using Material-UI's ThemeProvider component.

#### Scenario: Wrap application with ThemeProvider

- **WHEN** the application initializes
- **THEN** the App component SHALL be wrapped with ThemeProvider
- **AND** the custom theme SHALL be passed as the theme prop
- **AND** all Material-UI components SHALL inherit the theme

#### Scenario: Apply theme to all pages

- **WHEN** any page renders
- **THEN** the page SHALL use theme colors via sx prop or styled components
- **AND** maintain consistent styling across the application
- **AND** apply dark mode by default

#### Scenario: Support dynamic theme access

- **WHEN** components need to access theme values
- **THEN** components SHALL use the useTheme() hook
- **AND** access theme properties (palette, spacing, typography)
- **AND** ensure type safety with TypeScript

### Requirement: Design Token Consistency

The UI SHALL use theme design tokens consistently across all components.

#### Scenario: Use theme spacing

- **WHEN** components need spacing (margins, padding)
- **THEN** components SHALL use theme.spacing() function
- **AND** use multiples of 8px (e.g., theme.spacing(1) = 8px, theme.spacing(2) = 16px)
- **AND** avoid hardcoded pixel values

#### Scenario: Use theme colors

- **WHEN** components need colors
- **THEN** components SHALL reference theme.palette properties
- **AND** avoid hardcoded hex values
- **AND** maintain consistency with the defined palette

#### Scenario: Use theme typography

- **WHEN** components display text
- **THEN** components SHALL use Typography component with defined variants
- **AND** reference theme.typography for custom text styles
- **AND** maintain consistent font sizes and weights

### Requirement: Dark Mode Default

The UI SHALL default to dark mode with warm, sophisticated styling.

#### Scenario: Set palette mode to dark

- **WHEN** the theme is created
- **THEN** the palette mode SHALL be set to "dark"
- **AND** Material-UI SHALL apply appropriate dark mode adjustments
- **AND** components SHALL render with dark backgrounds by default

#### Scenario: Maintain readability

- **WHEN** dark mode is active
- **THEN** text SHALL have sufficient contrast (WCAG AA compliance)
- **AND** primary text (#F1F5F9) SHALL be readable on dark backgrounds
- **AND** secondary text (#94A3B8) SHALL provide visual hierarchy

#### Scenario: Optimize for streaming UI

- **WHEN** displaying media content
- **THEN** dark backgrounds SHALL enhance poster visibility
- **AND** minimize eye strain during browsing
- **AND** match user expectations from streaming platforms
