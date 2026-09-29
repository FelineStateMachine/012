// Docusaurus's Mermaid component, swizzled so diagrams are drawn in 012's
// palette: Mermaid's base theme, its variables read from the colors
// custom.css defines for the color mode shown, so the palette has one
// home. Mermaid needs literal colors, not CSS variables, which is why
// they are read from the page rather than passed through.

import React, {useEffect, useMemo, useRef, useState, type ReactNode} from 'react';
import ErrorBoundary from '@docusaurus/ErrorBoundary';
import {ErrorBoundaryErrorMessageFallback, useColorMode} from '@docusaurus/theme-common';
import {
  MermaidContainerClassName,
  useMermaidRenderResult,
} from '@docusaurus/theme-mermaid/client';
import type {MermaidConfig, RenderResult} from 'mermaid';

type Props = {value: string};

// The custom properties the diagrams are drawn with, as custom.css
// names them without the leading dashes.
const vars = [
  'ansi-black',
  'ansi-red',
  'ansi-green',
  'ansi-yellow',
  'ansi-blue',
  'ansi-magenta',
  'ansi-cyan',
  'ansi-bright-black',
  'term-bg',
  'term-fg',
  'role-accent-bg',
  'role-accent-fg',
  'role-header-bg',
  'role-header-fg',
  'role-muted',
  'role-frame',
  'role-grid',
  'role-panel-bg',
  'font-mono',
] as const;

type Palette = Record<(typeof vars)[number], string>;

function readPalette(): Palette {
  const style = getComputedStyle(document.documentElement);
  const p = {} as Palette;
  for (const v of vars) {
    p[v] = style.getPropertyValue(`--${v}`).trim();
  }
  return p;
}

// Nodes are cells: the panel's background framed in the grid's frame
// color. Actors and composite states are header bands, notes the cyan
// of where you are, and lines the muted text color.
function themeVariables(p: Palette, dark: boolean): Record<string, unknown> {
  return {
    darkMode: dark,
    background: p['term-bg'],
    fontFamily: p['font-mono'],
    fontSize: '14px',
    // Square, flat boxes, as 012's frames are: no rounding, shadow or
    // gradient.
    radius: 0,
    dropShadow: 'none',
    useGradient: false,
    primaryColor: p['role-panel-bg'],
    primaryTextColor: p['term-fg'],
    primaryBorderColor: p['role-frame'],
    secondaryColor: p['role-header-bg'],
    secondaryTextColor: p['role-header-fg'],
    secondaryBorderColor: p['role-frame'],
    tertiaryColor: p['term-bg'],
    tertiaryTextColor: p['term-fg'],
    tertiaryBorderColor: p['role-grid'],
    textColor: p['term-fg'],
    lineColor: p['role-muted'],
    arrowheadColor: p['role-muted'],
    edgeLabelBackground: p['term-bg'],
    clusterBkg: p['term-bg'],
    clusterBorder: p['role-frame'],
    titleColor: p['term-fg'],
    noteBkgColor: p['role-accent-bg'],
    noteTextColor: p['role-accent-fg'],
    noteBorderColor: p['role-accent-bg'],
    actorBkg: p['role-header-bg'],
    actorTextColor: p['role-header-fg'],
    actorBorder: p['role-header-bg'],
    actorLineColor: p['role-frame'],
    signalColor: p['term-fg'],
    signalTextColor: p['term-fg'],
    labelBoxBkgColor: p['role-header-bg'],
    labelBoxBorderColor: p['role-header-bg'],
    labelTextColor: p['role-header-fg'],
    loopTextColor: p['term-fg'],
    activationBkgColor: p['role-panel-bg'],
    activationBorderColor: p['role-frame'],
    sequenceNumberColor: p['role-accent-fg'],
    stateBkg: p['role-panel-bg'],
    stateLabelColor: p['term-fg'],
    compositeBackground: p['term-bg'],
    compositeTitleBackground: p['role-header-bg'],
    compositeBorder: p['role-frame'],
    altBackground: p['role-panel-bg'],
    transitionColor: p['role-muted'],
    transitionLabelColor: p['term-fg'],
    labelBackgroundColor: p['term-bg'],
    specialStateColor: p['term-fg'],
    innerEndBackground: p['role-accent-bg'],
    errorBkgColor: p['ansi-red'],
    errorTextColor: p['term-bg'],
  };
}

function useMermaidConfig(): MermaidConfig | undefined {
  // The mode is read from the page's data-theme attribute, which is set
  // before colorMode changes. colorMode only makes this render again: while
  // the page hydrates it reports the default mode first, and a config
  // made from it would draw every diagram twice at once, the two drawings
  // clobbering each other. On the server there is no page, and Mermaid
  // draws only in the browser.
  useColorMode();
  const mode = typeof document === 'undefined' ? undefined : document.documentElement.dataset.theme;
  return useMemo(() => {
    if (mode === undefined) {
      return undefined;
    }
    const palette = readPalette();
    // The font is also set outside the theme: diagrams that size boxes
    // by measuring their text (sequence diagrams) measure it in these,
    // and text measured in another font overflows its box.
    return {
      startOnLoad: false,
      theme: 'base',
      fontFamily: palette['font-mono'],
      fontSize: 14,
      themeVariables: themeVariables(palette, mode === 'dark'),
      flowchart: {curve: 'linear', htmlLabels: true},
    } as MermaidConfig;
  }, [mode]);
}

function MermaidRenderResult({renderResult}: {renderResult: RenderResult}): ReactNode {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (ref.current) {
      renderResult.bindFunctions?.(ref.current);
    }
  }, [renderResult]);
  return (
    <div
      ref={ref}
      className={`${MermaidContainerClassName} mermaid-012`}
      // eslint-disable-next-line react/no-danger
      dangerouslySetInnerHTML={{__html: renderResult.svg}}
    />
  );
}

function MermaidRenderer({value}: Props): ReactNode {
  const config = useMermaidConfig();
  const renderResult = useMermaidRenderResult({text: value, config});
  if (renderResult === null) {
    return null;
  }
  return <MermaidRenderResult renderResult={renderResult} />;
}

// Mermaid sizes boxes by measuring their text, so it draws once the
// site's fonts have loaded; measured in a fallback font, text overflows.
function useFontsReady(): boolean {
  const [ready, setReady] = useState(false);
  useEffect(() => {
    let live = true;
    document.fonts.ready.then(() => live && setReady(true));
    return () => {
      live = false;
    };
  }, []);
  return ready;
}

export default function Mermaid(props: Props): ReactNode {
  const ready = useFontsReady();
  return (
    <ErrorBoundary fallback={(params) => <ErrorBoundaryErrorMessageFallback {...params} />}>
      {ready && <MermaidRenderer {...props} />}
    </ErrorBoundary>
  );
}
