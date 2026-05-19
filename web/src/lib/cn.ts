import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/**
 * Classnames helper — combines clsx (conditional joining) with tailwind-merge
 * (deduplicates conflicting Tailwind classes). Standard shadcn convention.
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
