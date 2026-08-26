"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { Moon, Sun } from "lucide-react";
import { Toaster } from "sonner";

import { Switch } from "@/components/ui";

type Theme = "light" | "dark";

type ThemeContextValue = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute("content", theme === "dark" ? "#09090b" : "#ffffff");
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>("light");

  useEffect(() => {
    const stored = window.localStorage.getItem("v5-theme");
    const initial: Theme = stored === "dark" ? "dark" : "light";
    setThemeState(initial);
    applyTheme(initial);
  }, []);

  const setTheme = (next: Theme) => {
    setThemeState(next);
    window.localStorage.setItem("v5-theme", next);
    applyTheme(next);
  };

  const value: ThemeContextValue = {
    theme,
    setTheme,
    toggleTheme: () => setTheme(theme === "dark" ? "light" : "dark"),
  };

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const value = useContext(ThemeContext);
  if (!value) throw new Error("useTheme must be used inside ThemeProvider");
  return value;
}

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();
  const dark = theme === "dark";
  return (
    <div className="theme-switch" title={dark ? "Qora tema" : "Oq tema"}>
      <Sun size={15} aria-hidden="true" />
      <Switch
        checked={dark}
        onCheckedChange={(checked) => setTheme(checked ? "dark" : "light")}
        aria-label="Oq va qora temani almashtirish"
      />
      <Moon size={15} aria-hidden="true" />
    </div>
  );
}

export function ThemeToaster() {
  const { theme } = useTheme();
  return <Toaster position="top-right" theme={theme} closeButton />;
}
