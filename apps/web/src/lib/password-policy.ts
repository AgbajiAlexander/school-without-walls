const commonPasswords = new Set([
  "password",
  "password123",
  "password123!",
  "admin12345!",
  "welcome123!",
  "letmein123!",
  "school12345!",
  "nigeria1234!",
  "qwerty12345!",
  "123456789012",
  "qwertyuiop123",
]);

export function passwordPolicy(password: string): { lengthValid: boolean; notCommon: boolean; valid: boolean } {
  const length = Array.from(password).length;
  const lengthValid = length >= 12 && length <= 128;
  const notCommon = !commonPasswords.has(password.trim().toLowerCase());
  return { lengthValid, notCommon, valid: lengthValid && notCommon };
}
