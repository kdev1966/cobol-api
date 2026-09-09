/// <reference types="vite/client" />

// Origine de l'API. Vide en developpement, ou Vite mandate /v1 vers le
// backend ; renseignee au build de production, ou le frontend est servi depuis
// une autre origine et doit donc viser l'API explicitement.
interface ImportMetaEnv {
  readonly VITE_API_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
