import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "IELTS Student",
    short_name: "IELTS Student",
    description: "Secure student learning and assessment workspace for Assessment Platform IELTS.",
    start_url: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#ffffff",
  };
}
