import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "IELTS Student",
    short_name: "IELTS Student",
    description: "Secure IELTS learning, vocabulary and assessment workspace for students.",
    start_url: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#ffffff",
  };
}
