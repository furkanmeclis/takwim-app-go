-- Every panel session and open sign-in code is dropped; customers sign in
-- again. Identities and their links (000047) stay.
DROP TABLE IF EXISTS customer_sessions;
DROP TABLE IF EXISTS customer_otp_challenges;
