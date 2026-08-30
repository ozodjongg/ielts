import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "IELTS Teacher",
    short_name: "IELTS Teacher",
    description: "Secure teacher vocabulary and homework workspace for the IELTS platform.",
    start_url: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#ffffff",
  };
}
