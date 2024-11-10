def create_black_pgm(filename):
    width = 16
    height = 16

    # PGM header
    header = f"P2\n{width} {height}\n255\n"

    # Black pixel data (0 represents black)
    pixel_data = " ".join(["0"] * (width * height))

    # Write to file
    with open(filename, "w") as f:
        f.write(header + pixel_data)

# Nom du fichier PGM
filename = "16x16.pgm"
create_black_pgm(filename)
print(f"Fichier {filename} créé avec succès.")
