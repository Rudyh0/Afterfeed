<script lang="ts">
  // The Afterfeed symbol: a timeline running into a horizon with the sun on it.
  interface Props {
    width?: number
  }
  let { width = 64 }: Props = $props()
</script>

<svg {width} height={Math.round((width * 39) / 132)} viewBox="0 0 132 39" fill="currentColor" aria-hidden="true">
  <rect x="0" y="18" width="12" height="4" rx="2" />
  <circle cx="23.4" cy="20" r="3.4" />
  <circle cx="38.2" cy="20" r="3.4" />
  <rect x="49.6" y="18" width="82.4" height="4" rx="2" />
  <path d="M73 20V18a18 18 0 0 1 36 0v2z" />
  <rect x="74" y="25.8" width="34" height="2.4" rx="1.2" />
  <rect x="79.5" y="30.8" width="23" height="2.4" rx="1.2" />
  <rect x="85.5" y="35.8" width="11" height="2.4" rx="1.2" />
</svg>
