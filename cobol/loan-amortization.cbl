       IDENTIFICATION DIVISION.
       PROGRAM-ID. LOAN-AMORTIZATION.

      *> Produit l'echeancier d'un pret, frais et assurance compris.
      *>
      *> L'arithmetique est en decimal a virgule fixe : les montants sont
      *> exacts au centime, ce qu'un flottant binaire ne garantit pas.
      *> Six invariants sont tenus par construction :
      *>   1. la somme des parts de capital egale le capital emprunte
      *>   2. la somme des echeances egale capital + interets
      *>   3. le solde apres la derniere echeance est nul
      *>   4. chaque ligne verifie echeance = capital + interets
      *>   5. chaque ligne verifie du = echeance + assurance
      *>   6. le total du egale la somme des echeances et des assurances
      *> La derniere echeance absorbe le residu d'arrondi, comme le veut
      *> la pratique bancaire.
      *>
      *> Un differe partiel peut preceder l'amortissement : pendant la
      *> franchise, l'emprunteur ne paie que les interets et l'assurance, le
      *> capital reste intact. L'amortissement se fait ensuite sur la duree
      *> restante. Les interets ne sont pas capitalises : un differe total le
      *> ferait, et demanderait de distinguer les interets payes de ceux
      *> ajoutes au capital.
      *>
      *> Un remboursement anticipe total peut etre simule : l'emprunteur solde
      *> le capital restant du a une echeance donnee, moyennant une indemnite.
      *> Celle-ci n'est pas encadree par la loi tunisienne mais par le contrat,
      *> les banques pratiquant couramment un a un et demi pour cent du capital
      *> restant du ; son taux est donc un parametre.
      *>
      *> Methodes d'amortissement :
      *>   A  annuite constante : l'echeance ne bouge pas, la part de
      *>      capital croit a mesure que les interets diminuent
      *>   C  capital constant  : la part de capital ne bouge pas,
      *>      l'echeance decroit
      *>   I  in fine           : interets seuls, capital rembourse en
      *>      totalite a la derniere echeance
      *>
      *> Assiettes d'assurance :
      *>   N  aucune assurance
      *>   I  capital initial   : prime constante sur toute la duree
      *>   R  capital restant du : prime decroissante
      *>
      *> Les montants sont en millimes : le dinar tunisien se divise en mille,
      *> d'ou trois decimales sur tous les champs monetaires.
      *>
      *> Le taux effectif moyen de la categorie de concours est fourni en
      *> entree ; sa lecture, qui depend d'un bareme semestriel publie par
      *> arrete, releve de l'appelant. Ce programme garde la regle legale :
      *> majorer d'un cinquieme et statuer. Un TEM nul signifie qu'aucune
      *> verification n'est demandee.
      *>
      *> Entree : une ligne de 92 caracteres sur stdin
      *>            capital         9(11)V999  positions  1-14
      *>            taux annuel     9(2)V9(6)  positions 15-22
      *>            duree mois      9(4)       positions 23-26
      *>            frais dossier   9(9)V999   positions 27-38
      *>            frais garantie  9(9)V999   positions 39-50
      *>            taux assurance  9(2)V9(6)  positions 51-58
      *>            TEM categorie   9(2)V99    positions 59-62
      *>                            (zero = aucune verification)
      *>            differe mois    9(3)       positions 63-65
      *>                            (zero = aucun differe)
      *>            mois anticipe   9(4)       positions 66-69
      *>                            (zero = aucun remboursement anticipe)
      *>            taux indemnite  9(2)V9(4)  positions 70-75
      *>            montant anticipe 9(11)V999 positions 76-89
      *>                            (zero = solde total, non partiel)
      *>            methode         X          position     90
      *>            assiette assur. X          position     91
      *>            mode anticipe   X          position     92
      *>                            (T total, D duree reduite,
      *>                             M echeance reduite)
      *>
      *> Sortie : enregistrements a largeur fixe, un par ligne.
      *>          "R" recapitulatif : echeances 9(4), premiere et derniere
      *>              echeance 9(11)V999, total interets, total assurance
      *>              9(13)V999, total frais 9(11)V999, total du et cout du
      *>              credit 9(13)V999, taeg 9(2)V9(4), taux d'usure
      *>              9(2)V9(4), conformite X, marge S9(2)V9(4) a signe
      *>              separe                                  -> 129 car.
      *>          "E" echeance      : numero 9(4), echeance, interets,
      *>              capital, assurance, du, solde 9(11)V999  -> 89 car.
      *>          "A" solde total   : mois 9(4), solde restant, indemnite
      *>              9(11)V999, total anticipe, total au terme, economie,
      *>              interets economises 9(13)V999            -> 97 car.
      *>              (emis seulement en mode T avec un mois non nul)
      *>          "P" remb. partiel : mois 9(4), montant, indemnite
      *>              9(11)V999, mode X, duree 9(4), echeance suivante
      *>              9(11)V999, total, total au terme, economie, interets
      *>              economises 9(13)V999                    -> 116 car.
      *>              (emis seulement en mode D ou M)
      *>
      *>          L'echeancier rendu reste celui du contrat, quel que soit le
      *>          remboursement anticipe simule : celui-ci est une decision de
      *>          l'emprunteur, pas une clause du pret.
      *>
      *>          JSON GENERATE n'est deliberement pas utilise : le paquet
      *>          GnuCOBOL des distributions est construit sans bibliotheque
      *>          JSON, et l'instruction compile alors sans rien produire, ni
      *>          erreur ni avertissement. La mise en forme revient a
      *>          l'appelant.
      *>
      *> Retour : 0 succes, 2 entree malformee, 3 parametres hors bornes,
      *>          4 methode ou assiette inconnue, 5 differe ou mois de
      *>          remboursement anticipe incompatible avec la duree.
      *>
      *> Le TAEG est le taux actuariel annuel qui egalise la valeur actuelle
      *> de ce que l'emprunteur verse au montant qu'il percoit reellement,
      *> soit le capital diminue des frais. Il ne se deduit d'aucune formule
      *> des lors que des frais et une assurance entrent dans les flux : il
      *> est resolu par dichotomie, en arithmetique decimale exacte.

       ENVIRONMENT DIVISION.

       DATA DIVISION.
       WORKING-STORAGE SECTION.

       01 WS-ENTREE             PIC X(92) VALUE SPACES.
       01 WS-ENTREE-CHAMPS REDEFINES WS-ENTREE.
          05 WS-E-CHIFFRES      PIC X(89).
          05 WS-E-METHODE       PIC X.
          05 WS-E-ASSIETTE      PIC X.
          05 WS-E-MODE-ANTIC    PIC X.
       01 WS-E-DETAIL REDEFINES WS-ENTREE.
          05 WS-E-CAPITAL       PIC 9(11)V999.
          05 WS-E-TAUX          PIC 9(2)V9(6).
          05 WS-E-DUREE         PIC 9(4).
          05 WS-E-FRAIS-DOSSIER PIC 9(9)V999.
          05 WS-E-FRAIS-GARANTIE PIC 9(9)V999.
          05 WS-E-TAUX-ASSUR    PIC 9(2)V9(6).
          05 WS-E-TEM           PIC 9(2)V99.
          05 WS-E-DIFFERE       PIC 9(3).
          05 WS-E-MOIS-ANTICIPE PIC 9(4).
          05 WS-E-TAUX-INDEM    PIC 9(2)V9(4).
      *> Capital rembourse par anticipation en cours de pret, nul quand
      *> l'operation est un solde total.
          05 WS-E-MONTANT-ANTIC PIC 9(11)V999.
          05 FILLER             PIC X(3).

       78 METHODE-ANNUITE       VALUE "A".
       78 METHODE-CAPITAL       VALUE "C".
       78 METHODE-IN-FINE       VALUE "I".
       78 ASSIETTE-AUCUNE       VALUE "N".
       78 ASSIETTE-INITIAL      VALUE "I".
       78 ASSIETTE-RESTANT      VALUE "R".
       78 CONFORME-OUI          VALUE "O".
       78 CONFORME-NON          VALUE "N".
       78 CONFORME-SANS-OBJET   VALUE "-".
      *> Suite donnee a un remboursement anticipe : solder la totalite du
      *> capital, ou en rembourser une part et raccourcir la duree, ou en
      *> rembourser une part et alleger l'echeance.
       78 MODE-TOTAL            VALUE "T".
       78 MODE-DUREE            VALUE "D".
       78 MODE-MENSUALITE       VALUE "M".

       01 WS-CONFORME           PIC X      VALUE "-".
       01 WS-MARGE              PIC S9(2)V99   VALUE 0.

       01 WS-TAUX-MENSUEL       PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-ASSUR-MENSUEL PIC 9V9(18)    VALUE 0.
       01 WS-FACTEUR            PIC 9(9)V9(18) VALUE 0.
       01 WS-MENSUALITE         PIC 9(11)V999  VALUE 0.
      *> Part de capital fixe de la methode a capital constant.
       01 WS-AMORT-FIXE         PIC 9(11)V999  VALUE 0.
      *> Nombre d'echeances effectivement amortissantes, la franchise deduite.
       01 WS-DUREE-AMORT        PIC 9(4)       VALUE 0.
       01 WS-FRAIS              PIC 9(11)V999  VALUE 0.
       01 WS-VERSE              PIC 9(11)V999  VALUE 0.

       01 WS-I                  PIC 9(4)       VALUE 0.
       01 WS-SOLDE              PIC S9(11)V999 VALUE 0.
       01 WS-INTERET            PIC 9(11)V999  VALUE 0.
       01 WS-PART-CAPITAL       PIC 9(11)V999  VALUE 0.
       01 WS-ECHEANCE           PIC 9(11)V999  VALUE 0.
       01 WS-ASSURANCE          PIC 9(11)V999  VALUE 0.
       01 WS-DU                 PIC 9(11)V999  VALUE 0.

       01 WS-CUM-INTERET        PIC 9(13)V999  VALUE 0.
       01 WS-CUM-ASSURANCE      PIC 9(13)V999  VALUE 0.
       01 WS-CUM-DU             PIC 9(13)V999  VALUE 0.
      *> Total verse jusqu'a l'echeance du remboursement anticipe.
       01 WS-CUM-DU-ANTICIPE    PIC 9(13)V999  VALUE 0.
       01 WS-CUM-INT-ANTICIPE   PIC 9(13)V999  VALUE 0.
       01 WS-SOLDE-ANTICIPE     PIC 9(11)V999  VALUE 0.
       01 WS-INDEMNITE          PIC 9(11)V999  VALUE 0.
       01 WS-TOTAL-ANTICIPE     PIC 9(13)V999  VALUE 0.
       01 WS-ECONOMIE           PIC 9(13)V999  VALUE 0.
       01 WS-INT-ECONOMISES     PIC 9(13)V999  VALUE 0.

       01 WS-ANTICIPE.
          05 FILLER             PIC X          VALUE "A".
          05 WS-A-MOIS          PIC 9(4)       VALUE 0.
          05 WS-A-SOLDE         PIC 9(11)V999  VALUE 0.
          05 WS-A-INDEMNITE     PIC 9(11)V999  VALUE 0.
          05 WS-A-TOTAL         PIC 9(13)V999  VALUE 0.
          05 WS-A-TOTAL-TERME   PIC 9(13)V999  VALUE 0.
          05 WS-A-ECONOMIE      PIC 9(13)V999  VALUE 0.
          05 WS-A-INT-ECONOMIE  PIC 9(13)V999  VALUE 0.
      *> Trajectoire contractuelle, conservee pour la comparer a celle qui
      *> suit un remboursement partiel.
       01 WS-TOTAL-TERME-REF    PIC 9(13)V999  VALUE 0.
       01 WS-INT-TERME-REF      PIC 9(13)V999  VALUE 0.
       01 WS-TOTAL-PARTIEL      PIC 9(13)V999  VALUE 0.
       01 WS-INDEM-PARTIELLE    PIC 9(11)V999  VALUE 0.
      *> Duree reellement parcourue : elle raccourcit quand un remboursement
      *> partiel est impute en duree reduite.
       01 WS-DUREE-EFFECTIVE    PIC 9(4)       VALUE 0.
       01 WS-MOIS-RESTANTS      PIC 9(4)       VALUE 0.
      *> Echeance et amortissement d'origine, restaures a chaque passe : le
      *> mode « mensualite reduite » les modifie en cours de deroulement.
       01 WS-MENSUALITE-INIT    PIC 9(11)V999  VALUE 0.
       01 WS-AMORT-FIXE-INIT    PIC 9(11)V999  VALUE 0.
       01 WS-PARTIEL-ACTIF      PIC X          VALUE "N".
      *> Ce qui sera du le mois suivant l'operation, assurance comprise.
      *> Toutes methodes confondues, c'est la reponse a « je paie combien
      *> desormais ». Nul quand l'operation solde le pret.
       01 WS-ECHEANCE-SUIVANTE  PIC 9(11)V999  VALUE 0.

       01 WS-PARTIEL.
          05 FILLER             PIC X          VALUE "P".
          05 WS-P-MOIS          PIC 9(4)       VALUE 0.
          05 WS-P-MONTANT       PIC 9(11)V999  VALUE 0.
          05 WS-P-INDEMNITE     PIC 9(11)V999  VALUE 0.
          05 WS-P-MODE          PIC X          VALUE "T".
          05 WS-P-DUREE         PIC 9(4)       VALUE 0.
          05 WS-P-ECHEANCE-SUIV PIC 9(11)V999  VALUE 0.
          05 WS-P-TOTAL         PIC 9(13)V999  VALUE 0.
          05 WS-P-TOTAL-TERME   PIC 9(13)V999  VALUE 0.
          05 WS-P-ECONOMIE      PIC 9(13)V999  VALUE 0.
          05 WS-P-INT-ECONOMIE  PIC 9(13)V999  VALUE 0.

       01 WS-COUT-CREDIT        PIC 9(13)V999  VALUE 0.

      *> Duree maximale acceptee, qui dimensionne la table des versements
      *> conservee pour la resolution du TAEG.
       78 DUREE-MAX             VALUE 600.
       78 ITERATIONS-TAEG       VALUE 40.

       01 WS-VERSEMENTS.
          05 WS-VERSEMENT       PIC 9(11)V999 OCCURS 600 TIMES.

       01 WS-J                  PIC 9(4)       VALUE 0.
       01 WS-ITER               PIC 9(3)       VALUE 0.
       01 WS-TAUX-BAS           PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-HAUT          PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-ESSAI         PIC 9V9(18)    VALUE 0.
       01 WS-ESCOMPTE           PIC 9V9(18)    VALUE 0.
       01 WS-VALEUR-ACTUELLE    PIC 9(13)V9(6) VALUE 0.
       01 WS-TEG                PIC 9(2)V99    VALUE 0.
       01 WS-SEUIL              PIC 9(2)V99    VALUE 0.

      *> Le deroulement sert deux fois : une passe muette pour totaliser,
      *> une passe emettrice. Un seul corps de boucle, donc une seule
      *> regle de calcul.
       01 WS-EMETTRE            PIC X          VALUE "N".
       01 WS-PREMIERE           PIC 9(11)V999  VALUE 0.
       01 WS-DERNIERE           PIC 9(11)V999  VALUE 0.

       01 WS-RECAP.
          05 FILLER             PIC X          VALUE "R".
          05 WS-R-ECHEANCES     PIC 9(4)       VALUE 0.
          05 WS-R-PREMIERE      PIC 9(11)V999  VALUE 0.
          05 WS-R-DERNIERE      PIC 9(11)V999  VALUE 0.
          05 WS-R-INTERETS      PIC 9(13)V999  VALUE 0.
          05 WS-R-ASSURANCE     PIC 9(13)V999  VALUE 0.
          05 WS-R-FRAIS         PIC 9(11)V999  VALUE 0.
          05 WS-R-TOTAL-DU      PIC 9(13)V999  VALUE 0.
          05 WS-R-COUT          PIC 9(13)V999  VALUE 0.
          05 WS-R-TEG           PIC 9(2)V99    VALUE 0.
          05 WS-R-TEM           PIC 9(2)V99    VALUE 0.
          05 WS-R-SEUIL         PIC 9(2)V99    VALUE 0.
          05 WS-R-CONFORME      PIC X          VALUE "-".
          05 WS-R-MARGE         PIC S9(2)V99 SIGN IS LEADING SEPARATE.

       01 WS-LIGNE.
          05 FILLER             PIC X          VALUE "E".
          05 WS-L-NUMERO        PIC 9(4)       VALUE 0.
          05 WS-L-ECHEANCE      PIC 9(11)V999  VALUE 0.
          05 WS-L-INTERETS      PIC 9(11)V999  VALUE 0.
          05 WS-L-CAPITAL       PIC 9(11)V999  VALUE 0.
          05 WS-L-ASSURANCE     PIC 9(11)V999  VALUE 0.
          05 WS-L-DU            PIC 9(11)V999  VALUE 0.
          05 WS-L-SOLDE         PIC 9(11)V999  VALUE 0.

       PROCEDURE DIVISION.

       MAIN-PROCEDURE.
           PERFORM LIRE-DEMANDE
           PERFORM PREPARER-CALCUL

           MOVE "N" TO WS-EMETTRE
           PERFORM DEROULER-ECHEANCIER
           PERFORM CALCULER-TEG
           PERFORM VERIFIER-TAUX-EXCESSIF
           PERFORM ECRIRE-RECAPITULATIF
           IF WS-E-MOIS-ANTICIPE NOT = 0
               IF WS-E-MODE-ANTIC = MODE-TOTAL
                   PERFORM CALCULER-ANTICIPE
                   PERFORM ECRIRE-ANTICIPE
               ELSE
                   PERFORM SIMULER-PARTIEL
                   PERFORM ECRIRE-PARTIEL
               END-IF
           END-IF

      *> L'echeancier rendu reste celui du contrat : un remboursement
      *> anticipe est une decision de l'emprunteur, pas une clause.
           MOVE "O" TO WS-EMETTRE
           PERFORM DEROULER-ECHEANCIER

           STOP RUN.

       LIRE-DEMANDE.
           ACCEPT WS-ENTREE

           IF WS-E-CHIFFRES IS NOT NUMERIC
               DISPLAY "entree malformee : 89 chiffres puis trois lettres"
                   UPON SYSERR
               MOVE 2 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-CAPITAL = 0 OR WS-E-DUREE = 0
               DISPLAY "capital et duree doivent etre non nuls"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-DUREE > DUREE-MAX
               DISPLAY "duree superieure a " DUREE-MAX " mois"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

      *> Il doit rester au moins une echeance pour amortir le capital.
           IF WS-E-DIFFERE >= WS-E-DUREE
               DISPLAY "le differe doit laisser au moins une echeance "
                   "amortissante" UPON SYSERR
               MOVE 5 TO RETURN-CODE
               STOP RUN
           END-IF

           COMPUTE WS-DUREE-AMORT = WS-E-DUREE - WS-E-DIFFERE

      *> Solder a la derniere echeance revient a aller au terme : le cas est
      *> accepte, l'economie est alors nulle. Au-dela, il n'a pas de sens.
           IF WS-E-MOIS-ANTICIPE > WS-E-DUREE
               DISPLAY "le remboursement anticipe depasse la duree"
                   UPON SYSERR
               MOVE 5 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-MODE-ANTIC NOT = MODE-TOTAL
              AND WS-E-MODE-ANTIC NOT = MODE-DUREE
              AND WS-E-MODE-ANTIC NOT = MODE-MENSUALITE
               DISPLAY "mode de remboursement anticipe inconnu : "
                   WS-E-MODE-ANTIC UPON SYSERR
               MOVE 6 TO RETURN-CODE
               STOP RUN
           END-IF

      *> Solder la totalite ne laisse rien a rembourser partiellement, et
      *> reciproquement : les deux operations s'excluent.
           IF WS-E-MODE-ANTIC = MODE-TOTAL
              AND WS-E-MONTANT-ANTIC NOT = 0
               DISPLAY "un solde total ne prend pas de montant" UPON SYSERR
               MOVE 6 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-MODE-ANTIC NOT = MODE-TOTAL
               IF WS-E-MONTANT-ANTIC = 0
                   DISPLAY "un remboursement partiel exige un montant"
                       UPON SYSERR
                   MOVE 6 TO RETURN-CODE
                   STOP RUN
               END-IF

      *> Il doit rester une echeance apres l'operation pour que raccourcir
      *> la duree ou alleger l'echeance ait un sens.
               IF WS-E-MOIS-ANTICIPE = 0
                  OR WS-E-MOIS-ANTICIPE >= WS-E-DUREE
                   DISPLAY "le remboursement partiel doit laisser au moins "
                       "une echeance" UPON SYSERR
                   MOVE 6 TO RETURN-CODE
                   STOP RUN
               END-IF

      *> Sans amortissement avant le terme, il n'y a pas de duree a
      *> raccourcir : le capital est du en une fois, a la fin.
               IF WS-E-MODE-ANTIC = MODE-DUREE
                  AND WS-E-METHODE = METHODE-IN-FINE
                   DISPLAY "la duree reduite est sans objet in fine"
                       UPON SYSERR
                   MOVE 6 TO RETURN-CODE
                   STOP RUN
               END-IF
           END-IF

           IF WS-E-METHODE NOT = METHODE-ANNUITE
              AND WS-E-METHODE NOT = METHODE-CAPITAL
              AND WS-E-METHODE NOT = METHODE-IN-FINE
               DISPLAY "methode inconnue : " WS-E-METHODE UPON SYSERR
               MOVE 4 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-ASSIETTE NOT = ASSIETTE-AUCUNE
              AND WS-E-ASSIETTE NOT = ASSIETTE-INITIAL
              AND WS-E-ASSIETTE NOT = ASSIETTE-RESTANT
               DISPLAY "assiette d'assurance inconnue : "
                   WS-E-ASSIETTE UPON SYSERR
               MOVE 4 TO RETURN-CODE
               STOP RUN
           END-IF

      *> Les frais sont verses au depart : ils diminuent ce que
      *> l'emprunteur percoit, sans reduire ce qu'il rembourse.
           COMPUTE WS-FRAIS =
               WS-E-FRAIS-DOSSIER + WS-E-FRAIS-GARANTIE

           IF WS-FRAIS >= WS-E-CAPITAL
               DISPLAY "les frais absorbent le capital" UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

           COMPUTE WS-VERSE = WS-E-CAPITAL - WS-FRAIS.

      *> Taux periodiques proportionnels : taux nominal annuel divise par
      *> douze. C'est la convention du taux nominal, distincte du taux
      *> actuariel equivalent.
       PREPARER-CALCUL.
           COMPUTE WS-TAUX-MENSUEL = WS-E-TAUX / 100 / 12
           COMPUTE WS-TAUX-ASSUR-MENSUEL = WS-E-TAUX-ASSUR / 100 / 12

           EVALUATE WS-E-METHODE
               WHEN METHODE-ANNUITE
      *> La mensualite se calcule sur les seules echeances amortissantes :
      *> le capital est intact a la sortie de la franchise.
                   IF WS-TAUX-MENSUEL = 0
      *> Sans interets la formule diviserait par zero.
                       COMPUTE WS-MENSUALITE ROUNDED =
                           WS-E-CAPITAL / WS-DUREE-AMORT
                   ELSE
                       COMPUTE WS-FACTEUR =
                           (1 + WS-TAUX-MENSUEL) ** WS-DUREE-AMORT
                       COMPUTE WS-MENSUALITE ROUNDED =
                           WS-E-CAPITAL * WS-TAUX-MENSUEL
                           / (1 - (1 / WS-FACTEUR))
                   END-IF

               WHEN METHODE-CAPITAL
                   COMPUTE WS-AMORT-FIXE ROUNDED =
                       WS-E-CAPITAL / WS-DUREE-AMORT

               WHEN METHODE-IN-FINE
      *> Aucun capital n'est rembourse avant la derniere echeance.
                   MOVE 0 TO WS-AMORT-FIXE
           END-EVALUATE

      *> Le mode « mensualite reduite » recalcule ces deux valeurs en cours
      *> de deroulement : chaque passe repart de celles du contrat.
           MOVE WS-MENSUALITE TO WS-MENSUALITE-INIT
           MOVE WS-AMORT-FIXE TO WS-AMORT-FIXE-INIT.

       DEROULER-ECHEANCIER.
           MOVE WS-E-CAPITAL TO WS-SOLDE
           MOVE 0 TO WS-CUM-INTERET
           MOVE 0 TO WS-CUM-ASSURANCE
           MOVE 0 TO WS-CUM-DU
           MOVE WS-E-DUREE TO WS-DUREE-EFFECTIVE
           MOVE 0 TO WS-ECHEANCE-SUIVANTE
           MOVE WS-MENSUALITE-INIT TO WS-MENSUALITE
           MOVE WS-AMORT-FIXE-INIT TO WS-AMORT-FIXE

           PERFORM VARYING WS-I FROM 1 BY 1
               UNTIL WS-I > WS-DUREE-EFFECTIVE

               COMPUTE WS-INTERET ROUNDED =
                   WS-SOLDE * WS-TAUX-MENSUEL

      *> La prime se calcule sur le solde avant remboursement.
               EVALUATE WS-E-ASSIETTE
                   WHEN ASSIETTE-AUCUNE
                       MOVE 0 TO WS-ASSURANCE
                   WHEN ASSIETTE-INITIAL
                       COMPUTE WS-ASSURANCE ROUNDED =
                           WS-E-CAPITAL * WS-TAUX-ASSUR-MENSUEL
                   WHEN ASSIETTE-RESTANT
                       COMPUTE WS-ASSURANCE ROUNDED =
                           WS-SOLDE * WS-TAUX-ASSUR-MENSUEL
               END-EVALUATE

               IF WS-I <= WS-E-DIFFERE
      *> Pendant la franchise, seuls les interets et l'assurance sont
      *> dus : le capital reste intact.
                   MOVE 0 TO WS-PART-CAPITAL
               ELSE
               IF WS-I = WS-DUREE-EFFECTIVE
      *> Quelle que soit la methode, la derniere echeance solde le
      *> capital restant : c'est elle qui absorbe le residu accumule
      *> par les arrondis mensuels.
                   MOVE WS-SOLDE TO WS-PART-CAPITAL
               ELSE
                   EVALUATE WS-E-METHODE
                       WHEN METHODE-ANNUITE
                           COMPUTE WS-PART-CAPITAL =
                               WS-MENSUALITE - WS-INTERET
                       WHEN METHODE-CAPITAL
                           MOVE WS-AMORT-FIXE TO WS-PART-CAPITAL
                       WHEN METHODE-IN-FINE
                           MOVE 0 TO WS-PART-CAPITAL
                   END-EVALUATE

      *> En duree reduite, l'echeance dont la part de capital couvre le
      *> solde est la derniere : elle l'absorbe et clot le pret.
                   IF WS-PARTIEL-ACTIF = "O"
                      AND WS-E-MODE-ANTIC = MODE-DUREE
                      AND WS-PART-CAPITAL >= WS-SOLDE
                       MOVE WS-SOLDE TO WS-PART-CAPITAL
                       MOVE WS-I TO WS-DUREE-EFFECTIVE
                   END-IF
               END-IF
               END-IF

               COMPUTE WS-ECHEANCE = WS-PART-CAPITAL + WS-INTERET
               COMPUTE WS-DU = WS-ECHEANCE + WS-ASSURANCE

               SUBTRACT WS-PART-CAPITAL FROM WS-SOLDE
               ADD WS-INTERET    TO WS-CUM-INTERET
               ADD WS-ASSURANCE  TO WS-CUM-ASSURANCE
               ADD WS-DU         TO WS-CUM-DU

               MOVE WS-DU TO WS-VERSEMENT(WS-I)

               IF WS-PARTIEL-ACTIF = "O"
                   AND WS-I = WS-E-MOIS-ANTICIPE + 1
                   MOVE WS-DU TO WS-ECHEANCE-SUIVANTE
               END-IF

      *> Etat a l'echeance du remboursement anticipe : ce qui a ete verse
      *> jusque-la, et ce qui reste a solder.
               IF WS-E-MOIS-ANTICIPE NOT = 0
                   AND WS-I = WS-E-MOIS-ANTICIPE
                   MOVE WS-CUM-DU      TO WS-CUM-DU-ANTICIPE
                   MOVE WS-CUM-INTERET TO WS-CUM-INT-ANTICIPE
                   MOVE WS-SOLDE       TO WS-SOLDE-ANTICIPE
               END-IF

      *> Le remboursement partiel s'impute apres l'echeance du mois, sur le
      *> capital restant. Il ne porte pas d'interet : il est verse le jour
      *> ou l'echeance l'est.
               IF WS-PARTIEL-ACTIF = "O"
                   AND WS-I = WS-E-MOIS-ANTICIPE
                   SUBTRACT WS-E-MONTANT-ANTIC FROM WS-SOLDE
                   IF WS-SOLDE = 0
                       MOVE WS-I TO WS-DUREE-EFFECTIVE
                   ELSE
                       IF WS-E-MODE-ANTIC = MODE-MENSUALITE
                           PERFORM RECALCULER-ECHEANCE
                       END-IF
                   END-IF
               END-IF

               IF WS-I = 1
                   MOVE WS-DU TO WS-PREMIERE
               END-IF
               IF WS-I = WS-DUREE-EFFECTIVE
                   MOVE WS-DU TO WS-DERNIERE
               END-IF

               IF WS-EMETTRE = "O"
                   PERFORM ECRIRE-LIGNE
               END-IF

           END-PERFORM

           COMPUTE WS-COUT-CREDIT =
               WS-CUM-INTERET + WS-CUM-ASSURANCE + WS-FRAIS.

      *> Dichotomie sur le taux periodique : la valeur actuelle des
      *> versements decroit quand le taux monte, l'encadrement se resserre
      *> donc de moitie a chaque tour.
      *>
      *> Les versements etant construits a partir du taux nominal, le taux
      *> recherche en est proche ; 0,25 par mois le majore largement, meme
      *> avec des frais et une assurance. Quarante tours ramenent alors
      *> l'incertitude a 2,3e-13, bien au-dela des quatre decimales rendues.
       CALCULER-TEG.
           MOVE 0 TO WS-TAUX-BAS
           MOVE 0.25 TO WS-TAUX-HAUT

           PERFORM VARYING WS-ITER FROM 1 BY 1
               UNTIL WS-ITER > ITERATIONS-TAEG
               COMPUTE WS-TAUX-ESSAI =
                   (WS-TAUX-BAS + WS-TAUX-HAUT) / 2
               PERFORM VALEUR-ACTUELLE-DES-VERSEMENTS
      *> L'emprunteur ne percoit que le capital diminue des frais.
               IF WS-VALEUR-ACTUELLE > WS-VERSE
                   MOVE WS-TAUX-ESSAI TO WS-TAUX-BAS
               ELSE
                   MOVE WS-TAUX-ESSAI TO WS-TAUX-HAUT
               END-IF
           END-PERFORM

      *> Le decret n° 2000-462 impose un taux annuel proportionnel au taux de
      *> periode : le taux mensuel resolu est multiplie par douze, et non
      *> capitalise. Le resultat s'exprime avec deux decimales.
           COMPUTE WS-TEG ROUNDED = WS-TAUX-ESSAI * 12 * 100.

       VALEUR-ACTUELLE-DES-VERSEMENTS.
           MOVE 0 TO WS-VALEUR-ACTUELLE
           MOVE 1 TO WS-ESCOMPTE

           PERFORM VARYING WS-J FROM 1 BY 1 UNTIL WS-J > WS-E-DUREE
               COMPUTE WS-ESCOMPTE = WS-ESCOMPTE / (1 + WS-TAUX-ESSAI)
               COMPUTE WS-VALEUR-ACTUELLE = WS-VALEUR-ACTUELLE
                   + WS-VERSEMENT(WS-J) * WS-ESCOMPTE
           END-PERFORM.

      *> Loi n° 99-64 du 15 juillet 1999 : est excessif tout pret dont le taux
      *> effectif global excede de plus du cinquieme le taux effectif moyen
      *> pratique au semestre precedent pour la meme categorie de concours.
      *> Le seuil est donc le TEM majore de vingt pour cent, arrondi a deux
      *> decimales comme les taux publies par arrete.
      *>
      *> L'appelant fournit le TEM, qui est une donnee trimestrielle ; la
      *> regle de majoration et le verdict restent ici. Un TEM nul signifie
      *> qu'aucune verification n'est demandee.
       VERIFIER-TAUX-EXCESSIF.
           IF WS-E-TEM = 0
               MOVE CONFORME-SANS-OBJET TO WS-CONFORME
               MOVE 0 TO WS-SEUIL
               MOVE 0 TO WS-MARGE
               EXIT PARAGRAPH
           END-IF

           COMPUTE WS-SEUIL ROUNDED = WS-E-TEM * 12 / 10

      *> « Excede de plus du cinquieme » : un TEG egal au seuil reste licite.
           COMPUTE WS-MARGE = WS-SEUIL - WS-TEG
           IF WS-TEG > WS-SEUIL
               MOVE CONFORME-NON TO WS-CONFORME
           ELSE
               MOVE CONFORME-OUI TO WS-CONFORME
           END-IF.

      *> L'emprunteur verse les echeances jusqu'au mois choisi, puis solde le
      *> capital restant du, augmente de l'indemnite. L'economie est ce qu'il
      *> aurait verse en allant au terme, moins ce qu'il verse ainsi.
       CALCULER-ANTICIPE.
           COMPUTE WS-INDEMNITE ROUNDED =
               WS-SOLDE-ANTICIPE * WS-E-TAUX-INDEM / 100

           COMPUTE WS-TOTAL-ANTICIPE =
               WS-CUM-DU-ANTICIPE + WS-SOLDE-ANTICIPE + WS-INDEMNITE

           IF WS-CUM-DU > WS-TOTAL-ANTICIPE
               COMPUTE WS-ECONOMIE = WS-CUM-DU - WS-TOTAL-ANTICIPE
           ELSE
      *> Une indemnite elevee peut rendre l'operation perdante.
               MOVE 0 TO WS-ECONOMIE
           END-IF

           COMPUTE WS-INT-ECONOMISES =
               WS-CUM-INTERET - WS-CUM-INT-ANTICIPE.

      *> Repartir l'echeance sur les mois restants, le capital ayant baisse.
      *> L'amortissement ne commence qu'apres la franchise : quand le
      *> remboursement tombe pendant celle-ci, les mois a repartir se
      *> comptent depuis sa sortie.
       RECALCULER-ECHEANCE.
           IF WS-E-MOIS-ANTICIPE > WS-E-DIFFERE
               COMPUTE WS-MOIS-RESTANTS =
                   WS-E-DUREE - WS-E-MOIS-ANTICIPE
           ELSE
               COMPUTE WS-MOIS-RESTANTS = WS-E-DUREE - WS-E-DIFFERE
           END-IF

           EVALUATE WS-E-METHODE
               WHEN METHODE-ANNUITE
                   IF WS-TAUX-MENSUEL = 0
                       COMPUTE WS-MENSUALITE ROUNDED =
                           WS-SOLDE / WS-MOIS-RESTANTS
                   ELSE
                       COMPUTE WS-FACTEUR =
                           (1 + WS-TAUX-MENSUEL) ** WS-MOIS-RESTANTS
                       COMPUTE WS-MENSUALITE ROUNDED =
                           WS-SOLDE * WS-TAUX-MENSUEL
                           / (1 - (1 / WS-FACTEUR))
                   END-IF

               WHEN METHODE-CAPITAL
                   COMPUTE WS-AMORT-FIXE ROUNDED =
                       WS-SOLDE / WS-MOIS-RESTANTS

      *> In fine, l'echeance ne porte que les interets : elle suit d'elle
      *> meme le capital restant, sans rien a repartir.
               WHEN METHODE-IN-FINE
                   CONTINUE
           END-EVALUATE.

      *> Compare la trajectoire modifiee a la trajectoire contractuelle.
      *> La passe contractuelle vient de s'achever : ses totaux sont mis de
      *> cote avant que la seconde passe ne les ecrase.
       SIMULER-PARTIEL.
           MOVE WS-CUM-DU      TO WS-TOTAL-TERME-REF
           MOVE WS-CUM-INTERET TO WS-INT-TERME-REF

           IF WS-E-MONTANT-ANTIC > WS-SOLDE-ANTICIPE
               DISPLAY "le montant rembourse depasse le capital restant"
                   UPON SYSERR
               MOVE 6 TO RETURN-CODE
               STOP RUN
           END-IF

           COMPUTE WS-INDEM-PARTIELLE ROUNDED =
               WS-E-MONTANT-ANTIC * WS-E-TAUX-INDEM / 100

           MOVE "O" TO WS-PARTIEL-ACTIF
           PERFORM DEROULER-ECHEANCIER
           MOVE "N" TO WS-PARTIEL-ACTIF

      *> Ce que coute la trajectoire modifiee : les echeances effectivement
      *> versees, le capital rembourse par anticipation et son indemnite.
           COMPUTE WS-TOTAL-PARTIEL = WS-CUM-DU
               + WS-E-MONTANT-ANTIC + WS-INDEM-PARTIELLE

      *> Une indemnite assez forte peut annuler le gain : l'economie est
      *> alors nulle, jamais negative.
           IF WS-TOTAL-TERME-REF > WS-TOTAL-PARTIEL
               COMPUTE WS-ECONOMIE =
                   WS-TOTAL-TERME-REF - WS-TOTAL-PARTIEL
           ELSE
               MOVE 0 TO WS-ECONOMIE
           END-IF

           COMPUTE WS-INT-ECONOMISES =
               WS-INT-TERME-REF - WS-CUM-INTERET.

       ECRIRE-PARTIEL.
           MOVE WS-E-MOIS-ANTICIPE TO WS-P-MOIS
           MOVE WS-E-MONTANT-ANTIC TO WS-P-MONTANT
           MOVE WS-INDEM-PARTIELLE TO WS-P-INDEMNITE
           MOVE WS-E-MODE-ANTIC    TO WS-P-MODE
           MOVE WS-DUREE-EFFECTIVE TO WS-P-DUREE
      *> Echeance qui suit l'operation : allegee en mode « mensualite
      *> reduite », inchangee quand c'est la duree qui raccourcit.
           MOVE WS-ECHEANCE-SUIVANTE TO WS-P-ECHEANCE-SUIV
           MOVE WS-TOTAL-PARTIEL   TO WS-P-TOTAL
           MOVE WS-TOTAL-TERME-REF TO WS-P-TOTAL-TERME
           MOVE WS-ECONOMIE        TO WS-P-ECONOMIE
           MOVE WS-INT-ECONOMISES  TO WS-P-INT-ECONOMIE
           DISPLAY WS-PARTIEL.

       ECRIRE-ANTICIPE.
           MOVE WS-E-MOIS-ANTICIPE TO WS-A-MOIS
           MOVE WS-SOLDE-ANTICIPE  TO WS-A-SOLDE
           MOVE WS-INDEMNITE       TO WS-A-INDEMNITE
           MOVE WS-TOTAL-ANTICIPE  TO WS-A-TOTAL
           MOVE WS-CUM-DU          TO WS-A-TOTAL-TERME
           MOVE WS-ECONOMIE        TO WS-A-ECONOMIE
           MOVE WS-INT-ECONOMISES  TO WS-A-INT-ECONOMIE
           DISPLAY WS-ANTICIPE.

       ECRIRE-RECAPITULATIF.
           MOVE WS-E-DUREE       TO WS-R-ECHEANCES
           MOVE WS-PREMIERE      TO WS-R-PREMIERE
           MOVE WS-DERNIERE      TO WS-R-DERNIERE
           MOVE WS-CUM-INTERET   TO WS-R-INTERETS
           MOVE WS-CUM-ASSURANCE TO WS-R-ASSURANCE
           MOVE WS-FRAIS         TO WS-R-FRAIS
           MOVE WS-CUM-DU        TO WS-R-TOTAL-DU
           MOVE WS-COUT-CREDIT   TO WS-R-COUT
           MOVE WS-TEG           TO WS-R-TEG
           MOVE WS-E-TEM         TO WS-R-TEM
           MOVE WS-SEUIL         TO WS-R-SEUIL
           MOVE WS-CONFORME      TO WS-R-CONFORME
           MOVE WS-MARGE         TO WS-R-MARGE

           DISPLAY WS-RECAP.

       ECRIRE-LIGNE.
           MOVE WS-I            TO WS-L-NUMERO
           MOVE WS-ECHEANCE     TO WS-L-ECHEANCE
           MOVE WS-INTERET      TO WS-L-INTERETS
           MOVE WS-PART-CAPITAL TO WS-L-CAPITAL
           MOVE WS-ASSURANCE    TO WS-L-ASSURANCE
           MOVE WS-DU           TO WS-L-DU
           MOVE WS-SOLDE        TO WS-L-SOLDE

           DISPLAY WS-LIGNE.
