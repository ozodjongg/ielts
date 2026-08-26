import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "IELTS Platform Admin",
    short_name: "IELTS Admin",
    description: "Secure platform administration for Assessment Platform IELTS.",
    start_url: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#ffffff",
  };
}
