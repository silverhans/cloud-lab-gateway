export function demoCourseId(): string {
  return import.meta.env.VITE_DEMO_COURSE_ID ?? "";
}

export function demoLabTemplateId(): string {
  return import.meta.env.VITE_DEMO_LAB_TEMPLATE_ID ?? "";
}

export function demoCheckTemplateId(): string {
  return import.meta.env.VITE_DEMO_CHECK_TEMPLATE_ID ?? "";
}
