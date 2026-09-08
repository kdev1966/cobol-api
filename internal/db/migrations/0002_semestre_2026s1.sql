-- Arrete du 28 juillet 2026 : taux effectifs moyens et seuils des taux
-- excessifs correspondants. Les seuils publies valent le taux effectif moyen
-- majore d'un cinquieme ; ils ne sont pas repris ici, le programme COBOL les
-- recalcule et une suite de tests verifie qu'il retrouve les valeurs publiees.

INSERT INTO taux_effectifs_moyens (categorie, semestre, tem, arrete, publie_le) VALUES
    ('leasing',                   '2026S1', 13.37, 'Arrete du 28 juillet 2026', '2026-07-28'),
    ('decouverts',                '2026S1', 12.29, 'Arrete du 28 juillet 2026', '2026-07-28'),
    ('gestion_des_dettes',        '2026S1', 11.78, 'Arrete du 28 juillet 2026', '2026-07-28'),
    ('credits_consommation',      '2026S1', 11.23, 'Arrete du 28 juillet 2026', '2026-07-28'),
    ('credits_logement',          '2026S1', 10.25, 'Arrete du 28 juillet 2026', '2026-07-28'),
    ('credits_moyen_terme',       '2026S1',  9.80, 'Arrete du 28 juillet 2026', '2026-07-28'),
    ('credits_long_terme',        '2026S1',  9.63, 'Arrete du 28 juillet 2026', '2026-07-28'),
    ('credits_court_terme',       '2026S1',  9.57, 'Arrete du 28 juillet 2026', '2026-07-28');
