import { useSyncExternalStore } from "react";

// Status-bar colors for the installed app: the light and dark --background
// tokens in style.css (index.html's inline theme script repeats them).
export const THEME_COLORS = { light: "#f8fbfc", dark: "#0d1116" };

export function setThemeColor(dark) {
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", dark ? THEME_COLORS.dark : THEME_COLORS.light);
}

// Chromium fires beforeinstallprompt once, early in the page's life, so it is
// captured at import time (main.jsx imports this module) and replayed to
// whichever component asks later.
let deferredPrompt = null;
let installed = false;
const listeners = new Set();
const notify = () => listeners.forEach((fn) => fn());

if (typeof window !== "undefined") {
  window.addEventListener("beforeinstallprompt", (event) => {
    event.preventDefault();
    deferredPrompt = event;
    notify();
  });
  window.addEventListener("appinstalled", () => {
    deferredPrompt = null;
    installed = true;
    notify();
  });
}

// registerServiceWorker registers public/sw.js in production builds only, so
// the Vite dev server and the Playwright specs never run behind a cache.
export function registerServiceWorker() {
  if (!import.meta.env.PROD || !("serviceWorker" in navigator)) return;
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch((err) => console.warn("service worker registration failed", err));
  });
}

function subscribe(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

function snapshot() {
  if (installed || isStandalone()) return "installed";
  if (deferredPrompt) return "prompt";
  if (!window.isSecureContext) return "insecure";
  if (isIOS()) return "ios";
  return "manual";
}

function isStandalone() {
  return window.matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
}

function isIOS() {
  return /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
}

// useInstallState reports how float can be installed on this device:
// "installed" (running as an app), "prompt" (the browser offers an install
// dialog; call promptInstall), "ios" (Safari's Add to Home Screen),
// "insecure" (plain HTTP on a non-localhost origin: browsers only install
// over HTTPS) or "manual" (use the browser's own menu).
export function useInstallState() {
  return useSyncExternalStore(subscribe, snapshot, () => "manual");
}

export async function promptInstall() {
  if (!deferredPrompt) return;
  const prompt = deferredPrompt;
  deferredPrompt = null;
  await prompt.prompt();
  const { outcome } = await prompt.userChoice;
  if (outcome === "accepted") installed = true;
  notify();
}
