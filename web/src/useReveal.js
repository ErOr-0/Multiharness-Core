import { useEffect } from "react";

// Marks [data-reveal] elements visible as they scroll into view. The CSS that
// hides them before they are visible only applies under
// prefers-reduced-motion: no-preference, so reduced-motion users (and print)
// always see everything.
export default function useReveal() {
  useEffect(() => {
    const elements = [...document.querySelectorAll("[data-reveal]")];
    if (typeof IntersectionObserver === "undefined") {
      elements.forEach((element) => element.classList.add("is-visible"));
      return;
    }
    document.documentElement.classList.add("reveal-ready");
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            entry.target.classList.add("is-visible");
            observer.unobserve(entry.target);
          }
        }
      },
      { rootMargin: "0px 0px -10% 0px" },
    );
    elements.forEach((element) => observer.observe(element));
    return () => observer.disconnect();
  }, []);
}
